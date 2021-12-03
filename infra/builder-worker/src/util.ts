export const safeParseInt = (s: string): number => {
  try {
    return parseInt(s, 10);
  } catch (error) {
    return 0;
  }
};

export const trimSuffix = (s: string, suffix: string): string =>
  s.endsWith(suffix) ? s.slice(0, -suffix.length) : s;

export const trimPrefix = (s: string, prefix: string): string =>
  s.startsWith(prefix) ? s.slice(prefix.length) : s;
