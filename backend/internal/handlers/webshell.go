package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
	"github.com/uc-package/genet/internal/auth"
	"github.com/uc-package/genet/internal/k8s"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
)

type createWebShellSessionRequest struct {
	Cols int `json:"cols"`
	Rows int `json:"rows"`
}

type webShellControlMessage struct {
	Type string `json:"type"`
	Cols uint16 `json:"cols,omitempty"`
	Rows uint16 `json:"rows,omitempty"`
}

type webShellOutputWriter struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *webShellOutputWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.conn.WriteMessage(websocket.BinaryMessage, p); err != nil {
		return 0, err
	}
	return len(p), nil
}

type webShellSizeQueue struct {
	ch chan *remotecommand.TerminalSize
}

func newWebShellSizeQueue(cols, rows int) *webShellSizeQueue {
	q := &webShellSizeQueue{ch: make(chan *remotecommand.TerminalSize, 8)}
	q.Push(uint16(normalizeWebShellCols(cols)), uint16(normalizeWebShellRows(rows)))
	return q
}

func (q *webShellSizeQueue) Push(cols, rows uint16) {
	size := &remotecommand.TerminalSize{Width: cols, Height: rows}
	select {
	case q.ch <- size:
	default:
		select {
		case <-q.ch:
		default:
		}
		q.ch <- size
	}
}

func (q *webShellSizeQueue) Next() *remotecommand.TerminalSize {
	size, ok := <-q.ch
	if !ok {
		return nil
	}
	return size
}

func (q *webShellSizeQueue) Close() {
	close(q.ch)
}

func (h *PodHandler) CreateWebShellSession(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	podID := c.Param("id")
	userIdentifier := k8s.GetUserIdentifier(username, email)
	namespace := k8s.GetNamespaceForUserIdentifier(userIdentifier)

	var req createWebShellSessionRequest
	if err := c.ShouldBindJSON(&req); err != nil && err != io.EOF {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的会话参数"})
		return
	}

	pod, err := h.getPod(c.Request.Context(), namespace, podID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Pod 不存在"})
		return
	}
	if pod.Status.Phase != corev1.PodRunning {
		c.JSON(http.StatusConflict, gin.H{"error": "Pod 未处于运行状态，暂时无法打开 Web Shell"})
		return
	}

	container := h.getPodDisplayInfo(pod).ContainerName
	if container == "" && len(pod.Spec.Containers) > 0 {
		container = pod.Spec.Containers[0].Name
	}

	session := h.sessions.Create(WebShellSessionSpec{
		PodID:          podID,
		Namespace:      namespace,
		UserIdentifier: userIdentifier,
		Container:      container,
		Shell:          "/bin/sh",
		Cols:           req.Cols,
		Rows:           req.Rows,
	})

	c.JSON(http.StatusCreated, WebShellSessionResponse{
		SessionID:    session.ID,
		WebSocketURL: fmt.Sprintf("/api/pods/%s/webshell/sessions/%s/ws", podID, session.ID),
		Container:    session.Container,
		Shell:        session.Shell,
		Cols:         session.Cols,
		Rows:         session.Rows,
		ExpiresAt:    session.ExpiresAt,
	})
}

func (h *PodHandler) DeleteWebShellSession(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	podID := c.Param("id")
	sessionID := c.Param("sessionId")

	session, ok := h.sessions.Get(sessionID)
	if !ok || session.PodID != podID || session.UserIdentifier != userIdentifier {
		c.JSON(http.StatusNotFound, gin.H{"error": "Web Shell 会话不存在"})
		return
	}

	h.sessions.Delete(sessionID)
	c.JSON(http.StatusOK, gin.H{"message": "Web Shell 会话已关闭"})
}

func (h *PodHandler) WebShellWebSocket(c *gin.Context) {
	username, _ := auth.GetUsername(c)
	email, _ := auth.GetEmail(c)
	userIdentifier := k8s.GetUserIdentifier(username, email)
	podID := c.Param("id")
	sessionID := c.Param("sessionId")

	session, ok := h.sessions.Get(sessionID)
	if !ok || session.PodID != podID || session.UserIdentifier != userIdentifier {
		c.JSON(http.StatusNotFound, gin.H{"error": "Web Shell 会话不存在或已过期"})
		return
	}

	conn, err := h.webShellUpgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	defer h.sessions.Delete(sessionID)

	if h.webShellStreamFn == nil {
		h.webShellStreamFn = h.streamWebShell
	}
	if err := h.webShellStreamFn(c.Request.Context(), session, conn); err != nil {
		_ = conn.WriteJSON(gin.H{"type": "error", "message": err.Error()})
	}
}

func (h *PodHandler) streamWebShell(ctx context.Context, session WebShellSession, conn *websocket.Conn) error {
	restConfig := h.k8sClient.GetRESTConfig()
	if restConfig == nil {
		return fmt.Errorf("kubernetes rest config unavailable")
	}

	req := h.k8sClient.GetClientset().CoreV1().RESTClient().Post().
		Resource("pods").
		Namespace(session.Namespace).
		Name(session.PodID).
		SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: session.Container,
			Command:   []string{session.Shell},
			Stdin:     true,
			Stdout:    true,
			Stderr:    true,
			TTY:       true,
		}, scheme.ParameterCodec)

	executor, err := remotecommand.NewSPDYExecutor(restConfig, http.MethodPost, req.URL())
	if err != nil {
		return err
	}

	stdinReader, stdinWriter := io.Pipe()
	defer stdinReader.Close()
	sizeQueue := newWebShellSizeQueue(session.Cols, session.Rows)
	defer sizeQueue.Close()
	outputWriter := &webShellOutputWriter{conn: conn}

	go func() {
		<-ctx.Done()
		_ = conn.Close()
		_ = stdinWriter.Close()
	}()

	go func() {
		defer stdinWriter.Close()
		for {
			msgType, payload, err := conn.ReadMessage()
			if err != nil {
				return
			}
			switch msgType {
			case websocket.BinaryMessage:
				if _, err := stdinWriter.Write(payload); err != nil {
					return
				}
			case websocket.TextMessage:
				var control webShellControlMessage
				if err := json.Unmarshal(payload, &control); err == nil && control.Type == "resize" {
					sizeQueue.Push(control.Cols, control.Rows)
					continue
				}
				if _, err := stdinWriter.Write(payload); err != nil {
					return
				}
			case websocket.CloseMessage:
				return
			}
		}
	}()

	return executor.Stream(remotecommand.StreamOptions{
		Stdin:             stdinReader,
		Stdout:            outputWriter,
		Stderr:            outputWriter,
		Tty:               true,
		TerminalSizeQueue: sizeQueue,
	})
}
