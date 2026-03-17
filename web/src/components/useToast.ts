import { useContext } from 'react';
import { ToastContext } from './toastContext';
import type { Toast, ToastVariant, ToastContextValue } from './toastContext';

export type { Toast, ToastVariant, ToastContextValue };
export { ToastProvider } from './Toast';

export function useToast(): ToastContextValue {
  const ctx = useContext(ToastContext);
  if (!ctx) {
    throw new Error('useToast must be used inside <ToastProvider>');
  }
  return ctx;
}
