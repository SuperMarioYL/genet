import { ThemeConfig } from 'antd';
import { lightTokens, darkTokens } from './tokens';

// 亮色主题 Ant Design 配置
export const lightAntdTheme: ThemeConfig = {
  token: {
    colorPrimary: lightTokens.accentPrimary,
    colorBgContainer: lightTokens.glassBg,
    colorBgElevated: lightTokens.glassBg,
    colorBgLayout: 'transparent',
    colorText: lightTokens.textPrimary,
    colorTextSecondary: lightTokens.textSecondary,
    colorBorder: lightTokens.borderColor,
    colorSuccess: lightTokens.success,
    colorWarning: lightTokens.warning,
    colorError: lightTokens.error,
    colorInfo: lightTokens.info,
    borderRadius: 12,
    fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
  },
  components: {
    Button: {
      borderRadius: 8,
      controlHeight: 36,
      fontWeight: 500,
    },
    Card: {
      borderRadiusLG: 16,
      boxShadowTertiary: lightTokens.shadowMd,
    },
    Modal: {
      borderRadiusLG: 20,
      contentBg: lightTokens.glassBg,
      headerBg: 'transparent',
    },
    Input: {
      borderRadius: 8,
      controlHeight: 36,
    },
    Select: {
      borderRadius: 8,
      controlHeight: 36,
    },
    Table: {
      borderRadius: 12,
      headerBg: 'rgba(255, 255, 255, 0.5)',
    },
    Tabs: {
      cardBg: 'transparent',
    },
    Message: {
      contentBg: lightTokens.glassBg,
    },
  },
};

// 暗色主题 Ant Design 配置
export const darkAntdTheme: ThemeConfig = {
  token: {
    colorPrimary: darkTokens.accentPrimary,
    colorBgContainer: darkTokens.glassBg,
    colorBgElevated: darkTokens.glassBg,
    colorBgLayout: 'transparent',
    colorText: darkTokens.textPrimary,
    colorTextSecondary: darkTokens.textSecondary,
    colorBorder: darkTokens.borderColor,
    colorSuccess: darkTokens.success,
    colorWarning: darkTokens.warning,
    colorError: darkTokens.error,
    colorInfo: darkTokens.info,
    borderRadius: 12,
    fontFamily: "'Inter', -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
  },
  components: {
    Button: {
      borderRadius: 8,
      controlHeight: 36,
      fontWeight: 500,
    },
    Card: {
      borderRadiusLG: 16,
      boxShadowTertiary: darkTokens.shadowMd,
    },
    Modal: {
      borderRadiusLG: 20,
      contentBg: darkTokens.glassBg,
      headerBg: 'transparent',
    },
    Input: {
      borderRadius: 8,
      controlHeight: 36,
    },
    Select: {
      borderRadius: 8,
      controlHeight: 36,
    },
    Table: {
      borderRadius: 12,
      headerBg: 'rgba(30, 30, 50, 0.5)',
    },
    Tabs: {
      cardBg: 'transparent',
    },
    Message: {
      contentBg: darkTokens.glassBg,
    },
  },
};
