import React, { useState } from 'react';
import { NavLink, Outlet } from 'react-router-dom';
import { useStore } from '../hooks/useStore';
import { ToastProvider } from './Toast';

// ─── Nav items ────────────────────────────────────────────────────────────────

interface NavItem {
  label: string;
  to: string;
  end?: boolean;
}

const NAV_ITEMS: NavItem[] = [
  { label: 'Dashboard', to: '/', end: true },
  { label: 'Queue', to: '/queue' },
  { label: 'Review', to: '/review' },
  { label: 'Batches', to: '/batches' },
];

const CONFIG_ITEMS: NavItem[] = [
  { label: 'Config', to: '/config', end: true },
  { label: 'Stash Instances', to: '/config/stash' },
  { label: 'Aliases', to: '/config/aliases' },
  { label: 'Template', to: '/config/template' },
];

// ─── API Key Modal ─────────────────────────────────────────────────────────────

interface ApiKeyModalProps {
  onSave: (key: string) => void;
}

const ApiKeyModal: React.FC<ApiKeyModalProps> = ({ onSave }) => {
  const [value, setValue] = useState('');

  const handleSubmit = (e: React.FormEvent) => {
    e.preventDefault();
    if (value.trim()) {
      onSave(value.trim());
    }
  };

  return (
    <div
      className="fixed inset-0 z-50 flex items-center justify-center"
      aria-modal="true"
      role="dialog"
      aria-labelledby="api-key-modal-title"
    >
      {/* Backdrop */}
      <div className="absolute inset-0 bg-black/50" aria-hidden="true" />

      {/* Panel */}
      <div className="relative bg-white rounded-xl shadow-xl p-6 max-w-sm w-full mx-4">
        <h2
          id="api-key-modal-title"
          className="text-base font-semibold text-gray-900 mb-1"
        >
          Enter API Key
        </h2>
        <p className="text-sm text-gray-500 mb-4">
          Enter your Stasharr API key to continue. This is stored locally in your browser.
        </p>

        <form onSubmit={handleSubmit} className="flex flex-col gap-3">
          <input
            type="password"
            autoFocus
            placeholder="API key"
            value={value}
            onChange={(e) => setValue(e.target.value)}
            className="w-full px-3 py-2 border border-gray-300 rounded-lg text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
          />
          <button
            type="submit"
            disabled={!value.trim()}
            className="px-4 py-2 text-sm font-medium text-white bg-blue-600 rounded-lg hover:bg-blue-700 disabled:opacity-50 disabled:cursor-not-allowed transition"
          >
            Save
          </button>
        </form>
      </div>
    </div>
  );
};

// ─── Sidebar ──────────────────────────────────────────────────────────────────

const Sidebar: React.FC = () => {
  const baseLinkClass =
    'block px-3 py-2 rounded-lg text-sm font-medium transition-colors';
  const activeLinkClass = `${baseLinkClass} bg-blue-50 text-blue-700`;
  const inactiveLinkClass = `${baseLinkClass} text-gray-600 hover:bg-gray-100 hover:text-gray-900`;

  const linkClass = ({ isActive }: { isActive: boolean }) =>
    isActive ? activeLinkClass : inactiveLinkClass;

  return (
    <nav className="w-56 flex-shrink-0 bg-white border-r border-gray-200 flex flex-col">
      {/* Brand */}
      <div className="px-4 py-4 border-b border-gray-100">
        <span className="text-lg font-bold text-gray-900 tracking-tight">Stasharr</span>
      </div>

      {/* Primary nav */}
      <div className="flex-1 overflow-y-auto px-2 py-3 space-y-0.5">
        {NAV_ITEMS.map((item) => (
          <NavLink key={item.to} to={item.to} end={item.end} className={linkClass}>
            {item.label}
          </NavLink>
        ))}

        {/* Config section */}
        <div className="mt-4 mb-1 px-3">
          <span className="text-xs font-semibold uppercase tracking-wider text-gray-400">
            Configuration
          </span>
        </div>
        {CONFIG_ITEMS.map((item) => (
          <NavLink key={item.to} to={item.to} end={item.end} className={linkClass}>
            {item.label}
          </NavLink>
        ))}
      </div>
    </nav>
  );
};

// ─── Layout ───────────────────────────────────────────────────────────────────

const Layout: React.FC = () => {
  const apiKey = useStore((s) => s.apiKey);
  const setApiKey = useStore((s) => s.setApiKey);

  const showApiKeyPrompt = !apiKey;

  return (
    <ToastProvider>
      <div className="flex h-screen overflow-hidden bg-gray-50">
        <Sidebar />

        {/* Main content */}
        <main className="flex-1 overflow-y-auto">
          <Outlet />
        </main>
      </div>

      {/* API key modal — shown when no key is configured */}
      {showApiKeyPrompt && <ApiKeyModal onSave={setApiKey} />}
    </ToastProvider>
  );
};

export default Layout;
