import React, { useState, useEffect } from 'react';
import dayjs from 'dayjs';
import duration from 'dayjs/plugin/duration';
import { Tag } from 'antd';

dayjs.extend(duration);

interface CountdownTimerProps {
  expiresAt: string;
  warningThreshold?: number; // 小时
}

const CountdownTimer: React.FC<CountdownTimerProps> = ({ 
  expiresAt, 
  warningThreshold = 1 
}) => {
  const [timeLeft, setTimeLeft] = useState('');
  const [isWarning, setIsWarning] = useState(false);

  useEffect(() => {
    const updateTimer = () => {
      const now = dayjs();
      const expires = dayjs(expiresAt);
      const diff = expires.diff(now);

      if (diff <= 0) {
        setTimeLeft('已过期');
        setIsWarning(true);
        return;
      }

      const duration = dayjs.duration(diff);
      const hours = Math.floor(duration.asHours());
      const minutes = duration.minutes();

      if (hours < warningThreshold) {
        setIsWarning(true);
      }

      if (hours > 0) {
        setTimeLeft(`${hours}小时 ${minutes}分钟`);
      } else {
        setTimeLeft(`${minutes}分钟`);
      }
    };

    updateTimer();
    const timer = setInterval(updateTimer, 60000); // 每分钟更新

    return () => clearInterval(timer);
  }, [expiresAt, warningThreshold]);

  return (
    <Tag color={isWarning ? 'orange' : 'blue'}>
      ⏰ 剩余 {timeLeft}
    </Tag>
  );
};

export default CountdownTimer;

