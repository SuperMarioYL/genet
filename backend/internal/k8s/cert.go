package k8s

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"time"

	"go.uber.org/zap"
	certificatesv1 "k8s.io/api/certificates/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
)

// UserCertificate 用户证书
type UserCertificate struct {
	CertificatePEM string // 证书 PEM 格式
	PrivateKeyPEM  string // 私钥 PEM 格式
	Username       string // 用户名
	ExpiresAt      time.Time
}

// GenerateUserCertificate 为用户生成客户端证书
// 使用 K8s CSR API 签发证书
func (c *Client) GenerateUserCertificate(ctx context.Context, username string, validityHours int) (*UserCertificate, error) {
	c.log.Info("Generating user certificate",
		zap.String("username", username),
		zap.Int("validityHours", validityHours))

	// 1. 生成 RSA 私钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		c.log.Error("Failed to generate private key", zap.Error(err))
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// 2. 创建 CSR
	csrTemplate := &x509.CertificateRequest{
		Subject: pkix.Name{
			CommonName:   username,
			Organization: []string{"genet-users"},
		},
	}

	csrDER, err := x509.CreateCertificateRequest(rand.Reader, csrTemplate, privateKey)
	if err != nil {
		c.log.Error("Failed to create CSR", zap.Error(err))
		return nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	csrPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "CERTIFICATE REQUEST",
		Bytes: csrDER,
	})

	// 3. 提交 CSR 到 K8s
	csrName := fmt.Sprintf("genet-user-%s-%d", username, time.Now().Unix())
	expirationSeconds := int32(validityHours * 3600)

	k8sCSR := &certificatesv1.CertificateSigningRequest{
		ObjectMeta: metav1.ObjectMeta{
			Name: csrName,
			Labels: map[string]string{
				"genet.io/user":    username,
				"genet.io/managed": "true",
			},
		},
		Spec: certificatesv1.CertificateSigningRequestSpec{
			Request:           csrPEM,
			SignerName:        "kubernetes.io/kube-apiserver-client",
			ExpirationSeconds: &expirationSeconds,
			Usages: []certificatesv1.KeyUsage{
				certificatesv1.UsageClientAuth,
			},
		},
	}

	clientset := c.GetClientset()
	createdCSR, err := clientset.CertificatesV1().CertificateSigningRequests().Create(ctx, k8sCSR, metav1.CreateOptions{})
	if err != nil {
		c.log.Error("Failed to create CSR in K8s",
			zap.String("csrName", csrName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to create CSR: %w", err)
	}

	c.log.Info("CSR created", zap.String("csrName", csrName))

	// 4. 批准 CSR
	createdCSR.Status.Conditions = append(createdCSR.Status.Conditions, certificatesv1.CertificateSigningRequestCondition{
		Type:           certificatesv1.CertificateApproved,
		Status:         "True",
		Reason:         "GenetApproved",
		Message:        "Approved by Genet for user " + username,
		LastUpdateTime: metav1.Now(),
	})

	_, err = clientset.CertificatesV1().CertificateSigningRequests().UpdateApproval(ctx, csrName, createdCSR, metav1.UpdateOptions{})
	if err != nil {
		c.log.Error("Failed to approve CSR",
			zap.String("csrName", csrName),
			zap.Error(err))
		// 清理 CSR
		_ = clientset.CertificatesV1().CertificateSigningRequests().Delete(ctx, csrName, metav1.DeleteOptions{})
		return nil, fmt.Errorf("failed to approve CSR: %w", err)
	}

	c.log.Info("CSR approved", zap.String("csrName", csrName))

	// 5. 等待证书签发
	var certificate []byte
	err = wait.PollImmediate(500*time.Millisecond, 30*time.Second, func() (bool, error) {
		csr, err := clientset.CertificatesV1().CertificateSigningRequests().Get(ctx, csrName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		if len(csr.Status.Certificate) > 0 {
			certificate = csr.Status.Certificate
			return true, nil
		}
		return false, nil
	})

	if err != nil {
		c.log.Error("Failed to wait for certificate",
			zap.String("csrName", csrName),
			zap.Error(err))
		// 清理 CSR
		_ = clientset.CertificatesV1().CertificateSigningRequests().Delete(ctx, csrName, metav1.DeleteOptions{})
		return nil, fmt.Errorf("failed to get signed certificate: %w", err)
	}

	c.log.Info("Certificate issued", zap.String("csrName", csrName))

	// 6. 清理 CSR（证书已获取，不再需要 CSR 资源）
	_ = clientset.CertificatesV1().CertificateSigningRequests().Delete(ctx, csrName, metav1.DeleteOptions{})

	// 7. 编码私钥为 PEM
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	return &UserCertificate{
		CertificatePEM: string(certificate),
		PrivateKeyPEM:  string(privateKeyPEM),
		Username:       username,
		ExpiresAt:      time.Now().Add(time.Duration(validityHours) * time.Hour),
	}, nil
}
