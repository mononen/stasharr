import { BrowserRouter, Routes, Route } from 'react-router-dom';
import { QueryClient, QueryClientProvider } from '@tanstack/react-query';
import Layout from './components/Layout';
import Dashboard from './pages/Dashboard';
import Queue from './pages/Queue';
import JobDetail from './pages/JobDetail';
import ReviewQueue from './pages/ReviewQueue';
import Batches from './pages/Batches';
import BatchDetail from './pages/BatchDetail';
import Config from './pages/Config';
import StashInstances from './pages/StashInstances';
import TemplateBuilder from './pages/TemplateBuilder';
import Aliases from './pages/Aliases';

const queryClient = new QueryClient();

export default function App() {
  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route element={<Layout />}>
            <Route path="/" element={<Dashboard />} />
            <Route path="/queue" element={<Queue />} />
            <Route path="/queue/:id" element={<JobDetail />} />
            <Route path="/review" element={<ReviewQueue />} />
            <Route path="/batches" element={<Batches />} />
            <Route path="/batches/:id" element={<BatchDetail />} />
            <Route path="/config" element={<Config />} />
            <Route path="/config/stash" element={<StashInstances />} />
            <Route path="/config/template" element={<TemplateBuilder />} />
            <Route path="/config/aliases" element={<Aliases />} />
          </Route>
        </Routes>
      </BrowserRouter>
    </QueryClientProvider>
  );
}
