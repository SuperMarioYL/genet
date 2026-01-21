package oidc

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"math/big"
	"sync"
)

// KeyManager RSA 密钥管理器
type KeyManager struct {
	privateKey *rsa.PrivateKey
	publicKey  *rsa.PublicKey
	keyID      string
	mu         sync.RWMutex
}

// NewKeyManager 创建密钥管理器
func NewKeyManager() *KeyManager {
	return &KeyManager{}
}

// GenerateKeys 生成新的 RSA 密钥对
func (km *KeyManager) GenerateKeys() error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// 生成 2048 位 RSA 密钥
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return fmt.Errorf("生成 RSA 密钥失败: %w", err)
	}

	km.privateKey = privateKey
	km.publicKey = &privateKey.PublicKey

	// 生成 Key ID
	km.keyID = generateKeyID()

	return nil
}

// LoadKeys 从 PEM 格式加载密钥
func (km *KeyManager) LoadKeys(privateKeyPEM, publicKeyPEM string) error {
	km.mu.Lock()
	defer km.mu.Unlock()

	// 解析私钥
	block, _ := pem.Decode([]byte(privateKeyPEM))
	if block == nil {
		return fmt.Errorf("无法解析私钥 PEM")
	}

	privateKey, err := x509.ParsePKCS1PrivateKey(block.Bytes)
	if err != nil {
		// 尝试 PKCS8 格式
		key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
		if err != nil {
			return fmt.Errorf("解析私钥失败: %w", err)
		}
		var ok bool
		privateKey, ok = key.(*rsa.PrivateKey)
		if !ok {
			return fmt.Errorf("密钥不是 RSA 类型")
		}
	}

	km.privateKey = privateKey
	km.publicKey = &privateKey.PublicKey
	km.keyID = generateKeyID()

	return nil
}

// GetPrivateKey 获取私钥
func (km *KeyManager) GetPrivateKey() *rsa.PrivateKey {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.privateKey
}

// GetPublicKey 获取公钥
func (km *KeyManager) GetPublicKey() *rsa.PublicKey {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.publicKey
}

// GetKeyID 获取 Key ID
func (km *KeyManager) GetKeyID() string {
	km.mu.RLock()
	defer km.mu.RUnlock()
	return km.keyID
}

// GetJWKS 获取 JWKS 格式的公钥
func (km *KeyManager) GetJWKS() map[string]interface{} {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.publicKey == nil {
		return map[string]interface{}{
			"keys": []interface{}{},
		}
	}

	// 将公钥转换为 JWK 格式
	jwk := map[string]interface{}{
		"kty": "RSA",
		"alg": "RS256",
		"use": "sig",
		"kid": km.keyID,
		"n":   base64URLEncode(km.publicKey.N.Bytes()),
		"e":   base64URLEncode(big.NewInt(int64(km.publicKey.E)).Bytes()),
	}

	return map[string]interface{}{
		"keys": []interface{}{jwk},
	}
}

// ExportPrivateKeyPEM 导出私钥为 PEM 格式
func (km *KeyManager) ExportPrivateKeyPEM() string {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.privateKey == nil {
		return ""
	}

	privateKeyBytes := x509.MarshalPKCS1PrivateKey(km.privateKey)
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: privateKeyBytes,
	})

	return string(privateKeyPEM)
}

// ExportPublicKeyPEM 导出公钥为 PEM 格式
func (km *KeyManager) ExportPublicKeyPEM() string {
	km.mu.RLock()
	defer km.mu.RUnlock()

	if km.publicKey == nil {
		return ""
	}

	publicKeyBytes, err := x509.MarshalPKIXPublicKey(km.publicKey)
	if err != nil {
		return ""
	}

	publicKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PUBLIC KEY",
		Bytes: publicKeyBytes,
	})

	return string(publicKeyPEM)
}

// generateKeyID 生成随机的 Key ID
func generateKeyID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

// base64URLEncode Base64 URL 编码（无填充）
func base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}
