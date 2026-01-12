import React, { useState } from 'react';
import { Button, message } from 'antd';
import { CopyOutlined, CheckOutlined } from '@ant-design/icons';

interface CopyButtonProps {
  text: string;
  label?: string;
  size?: 'small' | 'middle' | 'large';
}

const CopyButton: React.FC<CopyButtonProps> = ({ text, label = '复制', size = 'small' }) => {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(text);
      setCopied(true);
      message.success('已复制到剪贴板');
      setTimeout(() => setCopied(false), 2000);
    } catch (err) {
      message.error('复制失败，请手动复制');
    }
  };

  return (
    <Button
      size={size}
      icon={copied ? <CheckOutlined /> : <CopyOutlined />}
      onClick={handleCopy}
      type={copied ? 'primary' : 'default'}
    >
      {copied ? '已复制' : label}
    </Button>
  );
};

export default CopyButton;

