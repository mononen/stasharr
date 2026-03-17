import { createContext } from 'react';

export type ToastVariant = 'success' | 'warning' | 'error';

export interface Toast {
  id: string;
  message: string;
  variant: ToastVariant;
}

export interface ToastContextValue {
  toasts: Toast[];
  toast: (message: string, variant?: ToastVariant) => void;
  dismiss: (id: string) => void;
}

export const ToastContext = createContext<ToastContextValue | null>(null);
