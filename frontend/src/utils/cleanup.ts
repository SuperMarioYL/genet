import dayjs from 'dayjs';

/**
 * 解析 cron 表达式，计算下一次清理时间，并返回智能时间标签
 *
 * 支持标准 5 段 cron: minute hour day month weekday
 * 例如: "0 23 * * *" -> 每天 23:00
 *
 * 智能标签规则：
 * - 今天 HH:mm（下次清理在今天）
 * - 明天 HH:mm
 * - 后天 HH:mm
 * - YYYY年MM月DD日 HH:mm（超过后天）
 */

export interface CleanupInfo {
  /** 下次清理的 dayjs 对象 */
  nextTime: dayjs.Dayjs;
  /** 智能时间标签，如 "今天 23:00" */
  label: string;
  /** 纯时间部分，如 "23:00" */
  timeStr: string;
}

/**
 * 从 cron 表达式解析小时和分钟
 * 只处理简单的固定时间 cron（minute hour * * * 形式）
 */
function parseCronTime(schedule: string): { hour: number; minute: number } | null {
  if (!schedule) return null;
  const parts = schedule.trim().split(/\s+/);
  if (parts.length < 2) return null;

  const minute = parseInt(parts[0], 10);
  const hour = parseInt(parts[1], 10);

  if (isNaN(minute) || isNaN(hour)) return null;
  if (minute < 0 || minute > 59 || hour < 0 || hour > 23) return null;

  return { hour, minute };
}

/**
 * 计算下一次清理时间
 */
export function getNextCleanupTime(schedule: string, timezone?: string): CleanupInfo | null {
  const cronTime = parseCronTime(schedule);
  if (!cronTime) return null;

  const now = dayjs();
  const timeStr = `${String(cronTime.hour).padStart(2, '0')}:${String(cronTime.minute).padStart(2, '0')}`;

  // 今天的清理时间
  let nextTime = now.hour(cronTime.hour).minute(cronTime.minute).second(0).millisecond(0);

  // 如果今天的清理时间已过，推到明天
  if (nextTime.isBefore(now)) {
    nextTime = nextTime.add(1, 'day');
  }

  const label = formatSmartDate(nextTime, timeStr);

  return { nextTime, label, timeStr };
}

/**
 * 智能格式化日期
 */
function formatSmartDate(target: dayjs.Dayjs, timeStr: string): string {
  const now = dayjs();
  const today = now.startOf('day');
  const targetDay = target.startOf('day');
  const diffDays = targetDay.diff(today, 'day');

  if (diffDays === 0) return `今天 ${timeStr}`;
  if (diffDays === 1) return `明天 ${timeStr}`;
  if (diffDays === 2) return `后天 ${timeStr}`;
  return `${target.format('YYYY年MM月DD日')} ${timeStr}`;
}

/**
 * 获取用于显示的清理时间文案
 * 返回 null 时表示无法解析，调用方可使用默认文案
 */
export function getCleanupLabel(schedule?: string, timezone?: string): string | null {
  if (!schedule) return null;
  const info = getNextCleanupTime(schedule, timezone);
  return info ? info.label : null;
}
