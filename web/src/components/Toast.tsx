import React, {
  useCallback,
  useEffect,
  useRef,
  useState,
} from 'react';
import { ToastContext } from './toastContext';
import type { Toast, ToastVariant } from './toastContext';

// ─── Single Toast Item ────────────────────────────────────────────────────────

const variantStyles: Record<ToastVariant, string> = {
  success: 'bg-green-600 text-white',
  warning: 'bg-amber-500 text-white',
  error: 'bg-red-600 text-white',
};

interface ToastItemProps {
  toast: Toast;
  onDismiss: (id: string) => void;
}

const ToastItem: React.FC<ToastItemProps> = ({ toast, onDismiss }) => {
  const timerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  useEffect(() => {
    if (toast.variant !== 'error') {
      timerRef.current = setTimeout(() => onDismiss(toast.id), 4000);
    }
    return () => {
      if (timerRef.current) clearTimeout(timerRef.current);
    };
  }, [toast.id, toast.variant, onDismiss]);

  return (
    <div
      role="alert"
      onClick={() => onDismiss(toast.id)}
      className={`flex items-start gap-2 px-4 py-3 rounded-lg shadow-lg cursor-pointer max-w-sm w-full select-none ${variantStyles[toast.variant]}`}
    >
      <span className="flex-1 text-sm">{toast.message}</span>
      <button
        onClick={(e) => {
          e.stopPropagation();
          onDismiss(toast.id);
        }}
        className="text-white/80 hover:text-white text-lg leading-none mt-0.5"
        aria-label="Dismiss"
      >
        ×
      </button>
    </div>
  );
};

// ─── Provider ─────────────────────────────────────────────────────────────────

let _idCounter = 0;

export const ToastProvider: React.FC<{ children: React.ReactNode }> = ({ children }) => {
  const [toasts, setToasts] = useState<Toast[]>([]);

  const dismiss = useCallback((id: string) => {
    setToasts((prev) => prev.filter((t) => t.id !== id));
  }, []);

  const toast = useCallback((message: string, variant: ToastVariant = 'success') => {
    const id = `toast-${++_idCounter}`;
    setToasts((prev) => [...prev, { id, message, variant }]);
  }, []);

  return (
    <ToastContext.Provider value={{ toasts, toast, dismiss }}>
      {children}
      {/* Toast container — fixed bottom-right, rendered here for portal-free simplicity */}
      <div
        aria-live="polite"
        className="fixed bottom-4 right-4 z-50 flex flex-col gap-2 items-end pointer-events-none"
      >
        {toasts.map((t) => (
          <div key={t.id} className="pointer-events-auto">
            <ToastItem toast={t} onDismiss={dismiss} />
          </div>
        ))}
      </div>
    </ToastContext.Provider>
  );
};

export default ToastProvider;
