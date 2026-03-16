import { create } from 'zustand';

interface AppState {
  apiKey: string;
  setApiKey: (key: string) => void;
}

export const useStore = create<AppState>((set) => ({
  apiKey: localStorage.getItem('stasharr_api_key') || '',
  setApiKey: (key: string) => {
    localStorage.setItem('stasharr_api_key', key);
    set({ apiKey: key });
  },
}));
