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

const characters = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789";
export const makeID = (): string => {
  var result = "";
  for (var i = 0; i < 20; i++) {
    result += characters.charAt(Math.floor(Math.random() * characters.length));
  }
  return result;
};

type InferArgs<T> = T extends (...t: [...infer Arg]) => any ? Arg : never;
type InferReturn<T> = T extends (...t: [...infer Arg]) => infer Res ? Res : never;

export function wrapError<TFunc extends (...args: any[]) => any>(
  func: TFunc,
): (...args: InferArgs<TFunc>) => [InferReturn<TFunc> | null, any] {
  return (...args: InferArgs<TFunc>) => {
    try {
      return [func(...args), null];
    } catch (error) {
      return [null, error];
    }
  };
}
