import React from 'react';

interface StatusBadgeProps {
  status: string;
}

function getStatusColor(status: string): string {
  // Blue — in-progress
  if (['submitted', 'resolving', 'searching', 'resolved'].includes(status)) {
    return 'bg-blue-100 text-blue-800 dark:bg-blue-900/30 dark:text-blue-300';
  }
  // Amber — awaiting review
  if (status === 'awaiting_review') {
    return 'bg-amber-100 text-amber-800 dark:bg-amber-900/30 dark:text-amber-300';
  }
  // Green — active/proceeding
  if (['approved', 'downloading', 'moving', 'scanning', 'download_complete', 'moved'].includes(status)) {
    return 'bg-green-100 text-green-800 dark:bg-green-900/30 dark:text-green-300';
  }
  // Gray — done or cancelled
  if (status === 'complete' || status === 'cancelled') {
    return 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300';
  }
  // Red — any *_failed status
  if (status.endsWith('_failed')) {
    return 'bg-red-100 text-red-800 dark:bg-red-900/30 dark:text-red-300';
  }
  // Fallback
  return 'bg-gray-100 text-gray-700 dark:bg-gray-700 dark:text-gray-300';
}

const StatusBadge: React.FC<StatusBadgeProps> = ({ status }) => {
  const colorClasses = getStatusColor(status);
  const label = status.replace(/_/g, ' ');

  return (
    <span
      className={`inline-flex items-center px-2 py-0.5 rounded text-xs font-medium capitalize ${colorClasses}`}
    >
      {label}
    </span>
  );
};

export default StatusBadge;
