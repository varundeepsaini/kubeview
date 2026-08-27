import "@testing-library/jest-dom/vitest";
import { vi } from "vitest";

// jsdom has no EventSource, but the watch hooks open one on mount. This inert
// stub lets streaming components mount without a live connection; tests that
// care about stream behavior can install their own.
class StubEventSource {
  static readonly CONNECTING = 0;
  static readonly OPEN = 1;
  static readonly CLOSED = 2;
  readyState: number = StubEventSource.CONNECTING;
  onopen: ((ev: Event) => void) | null = null;
  onmessage: ((ev: MessageEvent) => void) | null = null;
  onerror: ((ev: Event) => void) | null = null;
  constructor(public url: string) {}
  addEventListener() {}
  removeEventListener() {}
  close() {
    this.readyState = StubEventSource.CLOSED;
  }
}

vi.stubGlobal("EventSource", StubEventSource);
