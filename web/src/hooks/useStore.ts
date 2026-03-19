import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AppState {
  apiKey: string;
  setApiKey: (key: string) => void;
  theme: 'dark' | 'light';
  toggleTheme: () => void;
  safeMode: boolean;
  toggleSafeMode: () => void;
}

export const useStore = create<AppState>()(
  persist(
    (set) => ({
      apiKey: '',
      setApiKey: (key: string) => set({ apiKey: key }),
      theme: 'dark',
      toggleTheme: () => set((s) => ({ theme: s.theme === 'dark' ? 'light' : 'dark' })),
      safeMode: false,
      toggleSafeMode: () => set((s) => ({ safeMode: !s.safeMode })),
    }),
    {
      name: 'stasharr_api_key',
      partialize: (state) => ({ apiKey: state.apiKey, theme: state.theme, safeMode: state.safeMode }),
    },
  ),
);

export default useStore;
