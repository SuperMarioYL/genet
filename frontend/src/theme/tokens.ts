// 设计 Token - 颜色、间距、阴影等

export const lightTokens = {
  // 背景色
  bgPrimary: '#f0f4f8',
  bgSecondary: '#e8eef5',
  bgGradient: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
  
  // 玻璃效果
  glassBg: 'rgba(255, 255, 255, 0.7)',
  glassBorder: 'rgba(255, 255, 255, 0.5)',
  glassShadow: 'rgba(31, 38, 135, 0.15)',
  glassHoverBg: 'rgba(255, 255, 255, 0.85)',
  
  // 文字颜色
  textPrimary: '#1a1a2e',
  textSecondary: '#4a5568',
  textMuted: '#718096',
  
  // 强调色
  accentPrimary: '#667eea',
  accentSecondary: '#764ba2',
  accentGradient: 'linear-gradient(135deg, #667eea 0%, #764ba2 100%)',
  
  // 状态颜色
  success: '#48bb78',
  warning: '#ed8936',
  error: '#f56565',
  info: '#4299e1',
  
  // 粒子颜色
  particleColor: '#667eea',
  particleLineColor: 'rgba(102, 126, 234, 0.3)',
  
  // 边框
  borderColor: 'rgba(0, 0, 0, 0.08)',
  borderRadius: '16px',
  borderRadiusSm: '8px',
  borderRadiusLg: '24px',
  
  // 阴影
  shadowSm: '0 2px 8px rgba(31, 38, 135, 0.08)',
  shadowMd: '0 8px 32px rgba(31, 38, 135, 0.12)',
  shadowLg: '0 16px 48px rgba(31, 38, 135, 0.15)',
  
  // Header
  headerBg: 'rgba(255, 255, 255, 0.8)',
  headerBorder: 'rgba(255, 255, 255, 0.5)',
};

export const darkTokens = {
  // 背景色
  bgPrimary: '#0f0f1a',
  bgSecondary: '#1a1a2e',
  bgGradient: 'linear-gradient(135deg, #1a1a2e 0%, #16213e 100%)',
  
  // 玻璃效果
  glassBg: 'rgba(30, 30, 50, 0.7)',
  glassBorder: 'rgba(255, 255, 255, 0.1)',
  glassShadow: 'rgba(0, 0, 0, 0.3)',
  glassHoverBg: 'rgba(40, 40, 70, 0.85)',
  
  // 文字颜色
  textPrimary: '#e2e8f0',
  textSecondary: '#a0aec0',
  textMuted: '#718096',
  
  // 强调色
  accentPrimary: '#00d4ff',
  accentSecondary: '#7c3aed',
  accentGradient: 'linear-gradient(135deg, #00d4ff 0%, #7c3aed 100%)',
  
  // 状态颜色
  success: '#68d391',
  warning: '#fbd38d',
  error: '#fc8181',
  info: '#63b3ed',
  
  // 粒子颜色
  particleColor: '#00d4ff',
  particleLineColor: 'rgba(0, 212, 255, 0.2)',
  
  // 边框
  borderColor: 'rgba(255, 255, 255, 0.08)',
  borderRadius: '16px',
  borderRadiusSm: '8px',
  borderRadiusLg: '24px',
  
  // 阴影
  shadowSm: '0 2px 8px rgba(0, 0, 0, 0.2)',
  shadowMd: '0 8px 32px rgba(0, 0, 0, 0.3)',
  shadowLg: '0 16px 48px rgba(0, 0, 0, 0.4)',
  
  // Header
  headerBg: 'rgba(30, 30, 50, 0.8)',
  headerBorder: 'rgba(255, 255, 255, 0.1)',
};

export type ThemeTokens = typeof lightTokens;
