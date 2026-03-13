import { ArrowLeftOutlined, ReloadOutlined } from '@ant-design/icons';
import { Alert, Button, Layout, Space, Tag, Typography, message } from 'antd';
import { FitAddon } from '@xterm/addon-fit';
import { Terminal } from '@xterm/xterm';
import '@xterm/xterm/css/xterm.css';
import React, { useEffect, useRef, useState } from 'react';
import { useNavigate, useParams } from 'react-router-dom';
import GlassCard from '../../components/GlassCard';
import ThemeToggle from '../../components/ThemeToggle';
import { createWebShellSession, deleteWebShellSession } from '../../services/api';
import './index.css';

const { Header, Content } = Layout;
const { Text } = Typography;

const DEFAULT_COLS = 120;
const DEFAULT_ROWS = 40;

type ConnectionState = 'connecting' | 'connected' | 'disconnected' | 'error';

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
  const socketRef = useRef<WebSocket | null>(null);
  const sessionIdRef = useRef<string | null>(null);
  const sessionClosedRef = useRef(false);
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting');
  const [statusText, setStatusText] = useState('正在创建终端会话...');

  useEffect(() => {
    if (!terminalContainerRef.current || terminalRef.current) {
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

    terminalRef.current = terminal;
    fitAddonRef.current = fitAddon;

    const sendResize = () => {
      const socket = socketRef.current;
      const activeTerminal = terminalRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN || !activeTerminal) {
        return;
      }

      const controlMessage: WebShellControlMessage = {
        type: 'resize',
        cols: activeTerminal.cols,
        rows: activeTerminal.rows,
      };
      socket.send(JSON.stringify(controlMessage));
    };

    const dataDisposable = terminal.onData((data) => {
      const socket = socketRef.current;
      if (!socket || socket.readyState !== WebSocket.OPEN) {
        return;
      }
      socket.send(data);
    });

    if (typeof ResizeObserver !== 'undefined') {
      resizeObserverRef.current = new ResizeObserver(() => {
        fitAddon.fit();
        sendResize();
      });
      resizeObserverRef.current.observe(terminalContainerRef.current);
    } else {
      const handleResize = () => {
        fitAddon.fit();
        sendResize();
      };
      window.addEventListener('resize', handleResize);
      resizeObserverRef.current = {
        disconnect: () => window.removeEventListener('resize', handleResize),
      } as ResizeObserver;
    }

    return () => {
      resizeObserverRef.current?.disconnect();
      resizeObserverRef.current = null;
      dataDisposable.dispose();
      terminal.dispose();
      terminalRef.current = null;
      fitAddonRef.current = null;
    };
  }, []);

  useEffect(() => {
    if (!id) {
      setConnectionState('error');
      setStatusText('缺少 Pod 标识，无法打开终端。');
      return;
    }

    let cancelled = false;
    const openSession = async () => {
      try {
        setConnectionState('connecting');
        setStatusText('正在创建终端会话...');
        const session = await createWebShellSession(id, {
          cols: DEFAULT_COLS,
          rows: DEFAULT_ROWS,
        });

        if (cancelled) {
          return;
        }

        sessionIdRef.current = session.sessionId;
        setStatusText(`正在连接 ${session.container} 容器...`);

        const socket = new WebSocket(toWebSocketURL(session.webSocketURL));
        socket.binaryType = 'arraybuffer';
        socketRef.current = socket;

        socket.onopen = () => {
          if (cancelled) {
            return;
          }
          setConnectionState('connected');
          setStatusText(`已连接 ${session.container}，Shell: ${session.shell}`);
          fitAddonRef.current?.fit();
          const terminal = terminalRef.current;
          if (terminal) {
            const controlMessage: WebShellControlMessage = {
              type: 'resize',
              cols: terminal.cols,
              rows: terminal.rows,
            };
            socket.send(JSON.stringify(controlMessage));
          }
        };

        socket.onmessage = async (event) => {
          const terminal = terminalRef.current;
          if (!terminal) {
            return;
          }
          const content = await decodeOutput(event.data);
          terminal.write(content);
        };

        socket.onclose = () => {
          socketRef.current = null;
          if (cancelled || sessionClosedRef.current) {
            return;
          }
          setConnectionState('disconnected');
          setStatusText('终端连接已关闭。');
        };

        socket.onerror = () => {
          if (cancelled) {
            return;
          }
          setConnectionState('error');
          setStatusText('终端连接失败，请刷新重试。');
        };
      } catch (error: any) {
        if (cancelled) {
          return;
        }
        setConnectionState('error');
        setStatusText(error.message || '创建终端会话失败');
        message.error(error.message || '创建终端会话失败');
      }
    };

    openSession();

    return () => {
      cancelled = true;
      sessionClosedRef.current = true;
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
          <Button icon={<ArrowLeftOutlined />} onClick={() => navigate(-1)} className="glass-button">
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
          <div className="web-shell-terminal" ref={terminalContainerRef} />
        </GlassCard>
      </Content>
    </Layout>
  );
};

export default WebShellPage;
