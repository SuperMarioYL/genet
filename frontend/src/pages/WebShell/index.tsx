import { ArrowLeftOutlined, ReloadOutlined } from '@ant-design/icons';
import { Alert, Button, Layout, Space, Tag, Typography, message } from 'antd';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import dayjs from 'dayjs';
import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import GlassCard from '../../components/GlassCard';
import StatusBadge from '../../components/StatusBadge';
import ThemeToggle from '../../components/ThemeToggle';
import { createWebShellSession, deleteWebShellSession, getPod } from '../../services/api';
import './index.css';

const { Header, Content } = Layout;
const { Text } = Typography;

const DEFAULT_COLS = 120;
const DEFAULT_ROWS = 40;
const MAX_CONNECT_ATTEMPTS = 5;
const CONNECT_RETRY_DELAY_MS = 800;
const CONNECT_STABLE_DELAY_MS = 300;

type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'error';

interface WebShellPodSummary {
  name: string;
  status: string;
  image?: string;
  gpuType?: string;
  gpuCount?: number;
  cpu?: string;
  memory?: string;
  nodeIP?: string;
  createdAt?: string;
}

interface WebShellControlMessage {
  type: 'resize';
  cols: number;
  rows: number;
}

const toWebSocketURL = (value: string) => {
  const baseURL = new URL(value, window.location.origin);
  const protocol = baseURL.protocol === 'https:' ? 'wss:' : 'ws:';
  return `${protocol}//${baseURL.host}${baseURL.pathname}${baseURL.search}`;
};

const decodeOutput = async (payload: Blob | ArrayBuffer | Uint8Array | string) => {
  if (typeof payload === 'string') {
    return payload;
  }
  if (payload instanceof Uint8Array) {
    return payload;
  }
  if (payload instanceof ArrayBuffer) {
    return new Uint8Array(payload);
  }
  return new Uint8Array(await payload.arrayBuffer());
};

const WebShellPage: React.FC = () => {
  const { id } = useParams<{ id: string }>();
  const navigate = useNavigate();
  const terminalContainerRef = useRef<HTMLDivElement | null>(null);
  const terminalRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const resizeObserverRef = useRef<ResizeObserver | null>(null);
  const resizeFrameRef = useRef<number | null>(null);
  const lastMeasuredRef = useRef({ width: 0, height: 0, cols: 0, rows: 0 });
  const socketRef = useRef<WebSocket | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const sessionClosedRef = useRef(false);
  const reconnectTimerRef = useRef<number | null>(null);
  const outputBufferRef = useRef<Array<string | Uint8Array>>([]);
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting');
  const [statusText, setStatusText] = useState('正在创建终端会话...');
  const [podSummary, setPodSummary] = useState<WebShellPodSummary | null>(null);
  const [showTerminal, setShowTerminal] = useState(false);
  const handleBack = () => {
    navigate(id ? `/pods/${id}` : '/');
  };

  useEffect(() => {
    if (!showTerminal || !terminalContainerRef.current || terminalRef.current) {
      return;
    }

    const terminal = new Terminal({
      cols: DEFAULT_COLS,
      rows: DEFAULT_ROWS,
      cursorBlink: true,
      fontFamily: 'SFMono-Regular, Consolas, Liberation Mono, Menlo, monospace',
      fontSize: 14,
      lineHeight: 1.3,
      theme: {
        background: '#111827',
      },
    });
    const fitAddon = new FitAddon();

    terminal.loadAddon(fitAddon);
    terminal.open(terminalContainerRef.current);
    fitAddon.fit();
    terminal.focus();
    lastMeasuredRef.current = {
      width: terminalContainerRef.current.clientWidth,
      height: terminalContainerRef.current.clientHeight,
      cols: terminal.cols,
      rows: terminal.rows,
    };

    terminalRef.current = terminal;
    fitAddonRef.current = fitAddon;

    for (const chunk of outputBufferRef.current) {
      terminal.write(chunk);
    }
    outputBufferRef.current = [];

    const syncTerminalSize = () => {
      const activeTerminal = terminalRef.current;
      const container = terminalContainerRef.current;
      if (!activeTerminal || !container) {
        return;
      }

      const width = container.clientWidth;
      const height = container.clientHeight;
      const lastMeasured = lastMeasuredRef.current;
      if (lastMeasured.width === width && lastMeasured.height === height) {
        return;
      }

      fitAddon.fit();

      if (
        lastMeasured.width === width &&
        lastMeasured.height === height &&
        lastMeasured.cols === activeTerminal.cols &&
        lastMeasured.rows === activeTerminal.rows
      ) {
        return;
      }

      lastMeasuredRef.current = {
        width,
        height,
        cols: activeTerminal.cols,
        rows: activeTerminal.rows,
      };

      const controlMessage: WebShellControlMessage = {
        type: 'resize',
        cols: activeTerminal.cols,
        rows: activeTerminal.rows,
      };

      const socket = socketRef.current;
      if (socket && socket.readyState === WebSocket.OPEN) {
        socket.send(JSON.stringify(controlMessage));
      }
    };

    const requestTerminalResize = () => {
      if (resizeFrameRef.current !== null) {
        cancelAnimationFrame(resizeFrameRef.current);
      }
      resizeFrameRef.current = window.requestAnimationFrame(() => {
        resizeFrameRef.current = null;
        syncTerminalSize();
      });
    };

    const dataDisposable = terminal.onData((data) => {
      const socket = socketRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        return;
      }
      socket.send(new Blob([data]));
    });

    if (typeof ResizeObserver !== 'undefined') {
      resizeObserverRef.current = new ResizeObserver(() => {
        requestTerminalResize();
      });
      resizeObserverRef.current.observe(terminalContainerRef.current);
    } else {
      const handleResize = () => {
        requestTerminalResize();
      };
      window.addEventListener('resize', handleResize);
      resizeObserverRef.current = {
        disconnect: () => window.removeEventListener('resize', handleResize),
      } as ResizeObserver;
    }

    requestTerminalResize();

    return () => {
      if (resizeFrameRef.current !== null) {
        cancelAnimationFrame(resizeFrameRef.current);
        resizeFrameRef.current = null;
      }
      resizeObserverRef.current?.disconnect();
      resizeObserverRef.current = null;
      dataDisposable.dispose();
      terminal.dispose();
      terminalRef.current = null;
      fitAddonRef.current = null;
    };
  }, [showTerminal]);

  useEffect(() => {
    if (!id) {
      setPodSummary(null);
      return;
    }

    let cancelled = false;
    const loadPodSummary = async () => {
      try {
        const pod: any = await getPod(id);
        if (cancelled) {
          return;
        }
        setPodSummary(pod as WebShellPodSummary);
      } catch {
        if (!cancelled) {
          setPodSummary(null);
        }
      }
    };

    loadPodSummary();

    return () => {
      cancelled = true;
    };
  }, [id]);

  useEffect(() => {
    if (!id) {
      setConnectionState('error');
      setStatusText('缺少 Pod 标识，无法打开终端。');
      return;
    }

    let cancelled = false;
    sessionClosedRef.current = false;
    setShowTerminal(false);
    outputBufferRef.current = [];

    const clearReconnectTimer = () => {
      if (reconnectTimerRef.current !== null) {
        window.clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }
    };

    const waitForRetry = async (ms: number) => {
      await new Promise<void>((resolve) => {
        reconnectTimerRef.current = window.setTimeout(() => {
          reconnectTimerRef.current = null;
          resolve();
        }, ms);
      });
    };

    const releaseSession = async (sessionId: string | null) => {
      if (!id || !sessionId) {
        return;
      }
      try {
        await deleteWebShellSession(id, sessionId);
      } catch {
        return;
      } finally {
        if (sessionIdRef.current === sessionId) {
          sessionIdRef.current = null;
        }
      }
    };

    const connectSocket = async (
      session: Awaited<ReturnType<typeof createWebShellSession>>,
      attempt: number,
    ): Promise<boolean> => {
      const socket = new WebSocket(toWebSocketURL(session.webSocketURL));
      socket.binaryType = 'arraybuffer';
      socketRef.current = socket;

      return await new Promise<boolean>((resolve) => {
        let settled = false;
        let stableConnected = false;
        let stableTimer: number | null = null;

        const finish = (result: boolean) => {
          if (settled) {
            return;
          }
          settled = true;
          if (stableTimer !== null) {
            window.clearTimeout(stableTimer);
            stableTimer = null;
          }
          resolve(result);
        };

        socket.onopen = () => {
          if (cancelled) {
            socket.close();
            finish(false);
            return;
          }
          setStatusText(`正在建立终端连接（第 ${attempt}/${MAX_CONNECT_ATTEMPTS} 次）...`);
          stableTimer = window.setTimeout(() => {
            if (cancelled || socket.readyState !== WebSocket.OPEN) {
              finish(false);
              return;
            }
            stableConnected = true;
            setConnectionState('connected');
            setStatusText(`已连接 ${session.container}，Shell: ${session.shell}`);
            setShowTerminal(true);
            finish(true);
          }, CONNECT_STABLE_DELAY_MS);
        };

        socket.onmessage = async (event) => {
          const content = await decodeOutput(event.data);
          const terminal = terminalRef.current;
          if (!terminal) {
            outputBufferRef.current.push(content);
            return;
          }
          terminal.write(content);
        };

        socket.onclose = () => {
          if (socketRef.current === socket) {
            socketRef.current = null;
          }
          if (cancelled || sessionClosedRef.current) {
            finish(false);
            return;
          }
          if (stableConnected) {
            setConnectionState('disconnected');
            setStatusText('终端连接已关闭。');
            return;
          }
          finish(false);
        };

        socket.onerror = () => {
          if (cancelled || stableConnected) {
            return;
          }
          setStatusText(`终端连接失败，准备重试（第 ${attempt}/${MAX_CONNECT_ATTEMPTS} 次）...`);
        };
      });
    };

    const openSession = async () => {
      let lastError = '终端连接失败，请稍后重试。';
      setConnectionState('connecting');

      for (let attempt = 1; attempt <= MAX_CONNECT_ATTEMPTS; attempt += 1) {
        let sessionId: string | null = null;

        try {
          setShowTerminal(false);
          setStatusText(`正在创建终端会话（第 ${attempt}/${MAX_CONNECT_ATTEMPTS} 次）...`);
          const session = await createWebShellSession(id, {
            cols: DEFAULT_COLS,
            rows: DEFAULT_ROWS,
          });

          if (cancelled) {
            return;
          }

          sessionId = session.sessionId;
          sessionIdRef.current = session.sessionId;
          setStatusText(`正在连接 ${session.container} 容器（第 ${attempt}/${MAX_CONNECT_ATTEMPTS} 次）...`);

          const connected = await connectSocket(session, attempt);
          if (connected) {
            return;
          }

          lastError = '终端连接已关闭。';
        } catch (error: any) {
          if (cancelled) {
            return;
          }
          lastError = error.message || '创建终端会话失败';
        }

        await releaseSession(sessionId);
        if (cancelled) {
          return;
        }

        if (attempt < MAX_CONNECT_ATTEMPTS) {
          setConnectionState('connecting');
          setStatusText(`终端连接失败，正在重试（第 ${attempt + 1}/${MAX_CONNECT_ATTEMPTS} 次）...`);
          await waitForRetry(CONNECT_RETRY_DELAY_MS);
          continue;
        }

        setConnectionState('error');
        setStatusText(lastError);
        message.error(lastError);
      }
    };

    openSession();

    return () => {
      cancelled = true;
      sessionClosedRef.current = true;
      clearReconnectTimer();
      socketRef.current?.close();
      socketRef.current = null;
      if (sessionIdRef.current) {
        deleteWebShellSession(id, sessionIdRef.current).catch(() => undefined);
        sessionIdRef.current = null;
      }
    };
  }, [id]);

  const statusType =
    connectionState === 'connected'
      ? 'success'
      : connectionState === 'error'
        ? 'error'
        : connectionState === 'disconnected'
          ? 'warning'
          : 'info';

  return (
    <Layout className="web-shell-layout">
      <Header className="web-shell-header glass-header">
        <Space size="middle">
          <Button icon={<ArrowLeftOutlined />} onClick={handleBack} className="glass-button">
            返回
          </Button>
          <div className="web-shell-title">
            <h2>Web Shell</h2>
            <Text className="subtitle">{id || '未知 Pod'}</Text>
          </div>
        </Space>
        <Space size="middle">
          <Tag color={connectionState === 'connected' ? 'green' : connectionState === 'error' ? 'red' : 'blue'}>
            {connectionState === 'connected' ? 'Connected' : connectionState === 'error' ? 'Error' : connectionState === 'disconnected' ? 'Closed' : 'Connecting'}
          </Tag>
          <ThemeToggle />
        </Space>
      </Header>
      <Content className="web-shell-content">
        <GlassCard hover={false} className="web-shell-card">
          <Alert
            className="web-shell-status"
            message={statusText}
            type={statusType}
            showIcon
            action={
              connectionState !== 'connected' ? (
                <Button size="small" icon={<ReloadOutlined />} onClick={() => window.location.reload()}>
                  重新连接
                </Button>
              ) : undefined
            }
          />
          {podSummary && (
            <div className="web-shell-summary">
              <div className="web-shell-summary-main">
                <div className="web-shell-summary-title">
                  <span className="label">Pod</span>
                  <span className="value">{podSummary.name}</span>
                </div>
                <StatusBadge status={podSummary.status} />
              </div>
              <div className="web-shell-summary-grid">
                <div className="summary-chip">
                  <span className="label">CPU / 内存</span>
                  <span className="value">{podSummary.cpu || '-'} 核 / {podSummary.memory || '-'}</span>
                </div>
                <div className="summary-chip">
                  <span className="label">GPU</span>
                  <span className="value">
                    {podSummary.gpuType
                      ? `${podSummary.gpuType} ×${podSummary.gpuCount ?? 0}`
                      : ((podSummary.gpuCount ?? 0) > 0 ? `GPU ×${podSummary.gpuCount}` : '无')}
                  </span>
                </div>
                <div className="summary-chip">
                  <span className="label">节点 IP</span>
                  <span className="value">{podSummary.nodeIP || '-'}</span>
                </div>
                <div className="summary-chip">
                  <span className="label">创建时间</span>
                  <span className="value">
                    {podSummary.createdAt ? dayjs(podSummary.createdAt).format('MM-DD HH:mm') : '-'}
                  </span>
                </div>
              </div>
            </div>
          )}
          {showTerminal ? (
            <div className="web-shell-terminal" ref={terminalContainerRef} />
          ) : (
            <div className="web-shell-terminal-placeholder">
              <div className="placeholder-title">正在建立 Web Shell 连接</div>
              <div className="placeholder-text">最多自动重试 {MAX_CONNECT_ATTEMPTS} 次，连接稳定后再展示终端界面。</div>
            </div>
          )}
        </GlassCard>
      </Content>
    </Layout>
  );
};

export default WebShellPage;
