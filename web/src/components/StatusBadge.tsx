import React from 'react';

interface StatusBadgeProps {
  status: string;
}

function getStatusColor(status: string): string {
  // Blue — in-progress
  if (['submitted', 'resolving', 'searching', 'resolved'].includes(status)) {
    return 'bg-blue-100 text-blue-800';
  }
  // Amber — awaiting review
  if (status === 'awaiting_review') {
    return 'bg-amber-100 text-amber-800';
  }
  // Green — active/proceeding
  if (['approved', 'downloading', 'moving', 'scanning', 'download_complete', 'moved'].includes(status)) {
    return 'bg-green-100 text-green-800';
  }
  // Gray — done or cancelled
  if (status === 'complete' || status === 'cancelled') {
    return 'bg-gray-100 text-gray-700';
  }
  // Red — any *_failed status
  if (status.endsWith('_failed')) {
    return 'bg-red-100 text-red-800';
  }
  // Fallback
  return 'bg-gray-100 text-gray-700';
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
