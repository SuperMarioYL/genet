import React, { createContext, useContext, useEffect, useState } from 'react';
import { lightTokens, darkTokens, ThemeTokens } from './tokens';

type ThemeMode = 'light' | 'dark';

interface ThemeContextType {
  mode: ThemeMode;
  tokens: ThemeTokens;
  toggleTheme: () => void;
  setTheme: (mode: ThemeMode) => void;
}

const ThemeContext = createContext<ThemeContextType | undefined>(undefined);

const THEME_STORAGE_KEY = 'genet-theme-mode';

export const ThemeProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [mode, setMode] = useState<ThemeMode>(() => {
    // 从 localStorage 读取，默认亮色
    const stored = localStorage.getItem(THEME_STORAGE_KEY);
    if (stored === 'dark' || stored === 'light') {
      return stored;
    }
    return 'light';
  });

  const tokens = mode === 'light' ? lightTokens : darkTokens;

  // 切换主题
  const toggleTheme = () => {
    setMode(prev => prev === 'light' ? 'dark' : 'light');
  };

  // 设置主题
  const setTheme = (newMode: ThemeMode) => {
    setMode(newMode);
  };

  // 同步到 localStorage 和 CSS 变量
  useEffect(() => {
    localStorage.setItem(THEME_STORAGE_KEY, mode);
    
    // 更新 CSS 变量
    const root = document.documentElement;
    Object.entries(tokens).forEach(([key, value]) => {
      root.style.setProperty(`--${camelToKebab(key)}`, value);
    });
    
    // 设置 data-theme 属性
    root.setAttribute('data-theme', mode);
  }, [mode, tokens]);

  return (
    <ThemeContext.Provider value={{ mode, tokens, toggleTheme, setTheme }}>
      {children}
    </ThemeContext.Provider>
  );
};

// 使用主题的 Hook
export const useTheme = () => {
  const context = useContext(ThemeContext);
  if (!context) {
    throw new Error('useTheme must be used within a ThemeProvider');
  }
  return context;
};

// 辅助函数：camelCase 转 kebab-case
function camelToKebab(str: string): string {
  return str.replace(/([a-z0-9])([A-Z])/g, '$1-$2').toLowerCase();
}
