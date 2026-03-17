import { StrictMode } from 'react'
import { createRoot } from 'react-dom/client'
import './index.css'
import App from './App.tsx'

// Apply theme class before React renders to prevent flash
try {
  const stored = localStorage.getItem('stasharr_api_key');
  const savedTheme = stored ? (JSON.parse(stored) as { theme?: string }).theme : undefined;
  const initialTheme = savedTheme ?? 'dark';
  document.documentElement.classList.toggle('dark', initialTheme === 'dark');
} catch {
  document.documentElement.classList.add('dark');
}

createRoot(document.getElementById('root')!).render(
  <StrictMode>
    <App />
  </StrictMode>,
)
