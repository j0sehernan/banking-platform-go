'use client';

import { useState } from 'react';
import { useActivityStore, ActivityEntry } from '@/lib/activityStore';

export function ActivityPanel() {
  const { entries, paused, clear, togglePause } = useActivityStore();

  return (
    <aside className="w-[420px] shrink-0 border-l border-ink-700 bg-ink-950 text-slate-100 h-screen overflow-y-auto font-mono text-xs">
      <header className="sticky top-0 z-10 bg-ink-900 px-3 py-3 border-b border-ink-700 flex justify-between items-center">
        <div>
          <div className="font-semibold text-sm">📡 Activity Log</div>
          <div className="text-[10px] text-slate-400 mt-0.5">
            {entries.length} eventos · request/response del front
          </div>
        </div>
        <div className="flex gap-2">
          <button
            onClick={togglePause}
            className="px-2 py-1 rounded bg-ink-800 hover:bg-ink-700 text-[10px]"
            title={paused ? 'Reanudar captura' : 'Pausar captura'}
          >
            {paused ? '▶' : '⏸'}
          </button>
          <button
            onClick={clear}
            className="px-2 py-1 rounded bg-ink-800 hover:bg-ink-700 text-[10px]"
            title="Limpiar log"
          >
            ✕
          </button>
        </div>
      </header>

      <div className="p-2 space-y-1.5">
        {entries.length === 0 && (
          <div className="text-slate-500 text-center py-8 text-[11px]">
            Aún no hay actividad. Usá la app para ver los requests acá.
          </div>
        )}
        {entries.map((entry) => (
          <Entry key={entry.id} entry={entry} />
        ))}
      </div>
    </aside>
  );
}

function Entry({ entry }: { entry: ActivityEntry }) {
  const [expanded, setExpanded] = useState(false);

  const statusColor = getStatusColor(entry.status);
  const methodColor = getMethodColor(entry.method);

  return (
    <div className="bg-ink-900 rounded border border-ink-800">
      <button
        onClick={() => setExpanded(!expanded)}
        className="w-full text-left px-2.5 py-1.5 hover:bg-ink-800/40 rounded"
      >
        <div className="flex items-center gap-2">
          <span className={`font-semibold ${methodColor}`}>{entry.method}</span>
          <span className={`text-[10px] px-1.5 py-0.5 rounded ${statusColor}`}>
            {entry.status === 'pending' && '...'}
            {entry.status === 'streaming' && 'SSE'}
            {entry.status === 'error' && 'ERR'}
            {typeof entry.status === 'number' && entry.status}
          </span>
          {entry.durationMs !== undefined && (
            <span className="text-slate-500 text-[10px] ml-auto">{entry.durationMs}ms</span>
          )}
        </div>
        <div className="text-slate-300 truncate mt-0.5">{entry.url}</div>
      </button>

      {expanded && (
        <div className="border-t border-ink-800 px-2.5 py-2 space-y-2">
          {entry.requestBody !== undefined && (
            <Section label="Request" color="text-blue-300">
              <pre className="whitespace-pre-wrap break-all">
                {JSON.stringify(entry.requestBody, null, 2)}
              </pre>
            </Section>
          )}
          {entry.responseBody !== undefined && (
            <Section label="Response" color="text-emerald-300">
              <pre className="whitespace-pre-wrap break-all">
                {JSON.stringify(entry.responseBody, null, 2)}
              </pre>
            </Section>
          )}
          {entry.sseChunks && entry.sseChunks.length > 0 && (
            <Section label={`SSE chunks (${entry.sseChunks.length})`} color="text-purple-300">
              <pre className="whitespace-pre-wrap break-all">{entry.sseChunks.join('')}</pre>
            </Section>
          )}
          {entry.errorMessage && (
            <Section label="Error" color="text-red-300">
              <div>{entry.errorMessage}</div>
            </Section>
          )}
        </div>
      )}
    </div>
  );
}

function Section({ label, color, children }: { label: string; color: string; children: React.ReactNode }) {
  return (
    <div>
      <div className={`text-[10px] uppercase tracking-wide mb-1 ${color}`}>{label}</div>
      <div className="bg-ink-950 border border-ink-800 rounded px-2 py-1 text-slate-300 text-[10px] max-h-48 overflow-y-auto">
        {children}
      </div>
    </div>
  );
}

function getStatusColor(status: ActivityEntry['status']): string {
  if (status === 'pending') return 'bg-yellow-900/40 text-yellow-300';
  if (status === 'streaming') return 'bg-purple-900/40 text-purple-300';
  if (status === 'error') return 'bg-red-900/40 text-red-300';
  if (typeof status === 'number') {
    if (status >= 200 && status < 300) return 'bg-emerald-900/40 text-emerald-300';
    if (status >= 400 && status < 500) return 'bg-orange-900/40 text-orange-300';
    if (status >= 500) return 'bg-red-900/40 text-red-300';
  }
  return 'bg-slate-700 text-slate-300';
}

function getMethodColor(method: string): string {
  switch (method) {
    case 'GET':
      return 'text-blue-400';
    case 'POST':
      return 'text-emerald-400';
    case 'PUT':
      return 'text-yellow-400';
    case 'DELETE':
      return 'text-red-400';
    default:
      return 'text-slate-300';
  }
}
