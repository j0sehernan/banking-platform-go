// Global store of technical activity for the side panel.
// Captures every HTTP request and response from the front, and the SSE
// chunks from the LLM chat. Turns the app into a technical demo tool
// alongside its functional UI.
import { create } from 'zustand';

export type ActivityEntry = {
  id: string;
  timestamp: number;
  method: string;
  url: string;
  status: number | 'pending' | 'streaming' | 'error';
  durationMs?: number;
  requestBody?: unknown;
  responseBody?: unknown;
  errorMessage?: string;
  // SSE chunks (for the chat)
  sseChunks?: string[];
};

type State = {
  entries: ActivityEntry[];
  paused: boolean;
  addEntry: (entry: Omit<ActivityEntry, 'id' | 'timestamp'>) => string;
  updateEntry: (id: string, patch: Partial<ActivityEntry>) => void;
  appendChunk: (id: string, chunk: string) => void;
  clear: () => void;
  togglePause: () => void;
};

export const useActivityStore = create<State>((set, get) => ({
  entries: [],
  paused: false,

  addEntry: (entry) => {
    if (get().paused) return '';
    const id = crypto.randomUUID();
    const newEntry: ActivityEntry = {
      ...entry,
      id,
      timestamp: Date.now(),
    };
    set((state) => ({ entries: [newEntry, ...state.entries].slice(0, 100) }));
    return id;
  },

  updateEntry: (id, patch) => {
    if (!id) return;
    set((state) => ({
      entries: state.entries.map((e) => (e.id === id ? { ...e, ...patch } : e)),
    }));
  },

  appendChunk: (id, chunk) => {
    if (!id) return;
    set((state) => ({
      entries: state.entries.map((e) =>
        e.id === id
          ? { ...e, sseChunks: [...(e.sseChunks || []), chunk] }
          : e
      ),
    }));
  },

  clear: () => set({ entries: [] }),
  togglePause: () => set((state) => ({ paused: !state.paused })),
}));
