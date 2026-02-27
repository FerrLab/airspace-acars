// Mock for @wailsio/runtime used in tests

export function Call(_options: unknown): Promise<unknown> {
  return Promise.resolve(null);
}

export const Events = {
  On: (_event: string, _callback: (...args: unknown[]) => void) => () => {},
  Off: (_event: string) => {},
  Emit: (_event: string, _data?: unknown) => {},
  Once: (_event: string, _callback: (...args: unknown[]) => void) => () => {},
};

export const Window = {
  SetTitle: (_title: string) => {},
  Fullscreen: () => {},
  UnFullscreen: () => {},
  Minimize: () => {},
  Maximise: () => {},
  UnMaximise: () => {},
  Center: () => {},
  Show: () => {},
  Hide: () => {},
  Close: () => {},
};
