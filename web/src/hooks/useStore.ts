import { create } from 'zustand';
import { persist } from 'zustand/middleware';

interface AppState {
  apiKey: string;
  setApiKey: (key: string) => void;
}

export const useStore = create<AppState>()(
  persist(
    (set) => ({
      apiKey: '',
      setApiKey: (key: string) => set({ apiKey: key }),
    }),
    {
      name: 'stasharr_api_key',
      partialize: (state) => ({ apiKey: state.apiKey }),
    },
  ),
);

export default useStore;
