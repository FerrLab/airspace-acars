import { Component, type ErrorInfo, type ReactNode } from "react";
import { captureError } from "@/lib/sentry";

interface Props {
  children: ReactNode;
}

interface State {
  failed: boolean;
}

/**
 * Catches a render crash so the window shows something other than a blank
 * page, and reports it. Without this a thrown error in any component leaves
 * the pilot staring at nothing mid-flight with no way to tell us why.
 */
export class ErrorBoundary extends Component<Props, State> {
  state: State = { failed: false };

  static getDerivedStateFromError(): State {
    return { failed: true };
  }

  componentDidCatch(error: Error, info: ErrorInfo) {
    captureError(error, { componentStack: info.componentStack });
  }

  render() {
    if (!this.state.failed) return this.props.children;

    return (
      <div className="flex h-screen flex-col items-center justify-center gap-4 p-8 text-center">
        <h1 className="text-lg font-semibold">Something broke in the interface</h1>
        <p className="text-muted-foreground max-w-md text-sm">
          Flight tracking keeps running in the background. Reloading the window
          usually brings the interface back.
        </p>
        <button
          className="bg-primary text-primary-foreground rounded-md px-4 py-2 text-sm"
          onClick={() => window.location.reload()}
        >
          Reload
        </button>
      </div>
    );
  }
}
