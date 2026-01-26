import React from 'react';
import { Card, CardProps } from 'antd';
import './GlassCard.css';

interface GlassCardProps extends CardProps {
  hover?: boolean;
  glow?: boolean;
  className?: string;
}

const GlassCard: React.FC<GlassCardProps> = ({
  hover = true,
  glow = false,
  className = '',
  children,
  ...props
}) => {
  const classes = [
    'glass-card-component',
    hover ? 'glass-card-hover' : '',
    glow ? 'glass-card-glow' : '',
    className,
  ].filter(Boolean).join(' ');

  return (
    <Card className={classes} {...props}>
      {children}
    </Card>
  );
};

export default GlassCard;
