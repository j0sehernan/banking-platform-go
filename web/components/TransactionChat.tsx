'use client';

import { useState, useRef, useEffect } from 'react';
import { API } from '@/lib/api';
import { useActivityStore } from '@/lib/activityStore';

type Message = {
  role: 'user' | 'assistant';
  content: string;
};

export function TransactionChat({ txId, onClose }: { txId: string; onClose: () => void }) {
  const [messages, setMessages] = useState<Message[]>([]);
  const [input, setInput] = useState('');
  const [streaming, setStreaming] = useState(false);
  const scrollRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    scrollRef.current?.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' });
  }, [messages]);

  const send = async () => {
    if (!input.trim() || streaming) return;
    const userMsg: Message = { role: 'user', content: input.trim() };
    const next = [...messages, userMsg];
    setMessages(next);
    setInput('');
    setStreaming(true);

    // Append assistant message vacío que se va llenando con cada chunk
    const assistantIdx = next.length;
    setMessages([...next, { role: 'assistant', content: '' }]);

    // Registrar en activity store
    const store = useActivityStore.getState();
    const entryId = store.addEntry({
      method: 'POST',
      url: '/chat',
      status: 'streaming',
      requestBody: { tx_id: txId, messages: next },
    });

    try {
      const res = await fetch(`${API.llm}/chat`, {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ tx_id: txId, messages: next }),
      });

      if (!res.ok || !res.body) {
        throw new Error(`HTTP ${res.status}`);
      }

      const reader = res.body.getReader();
      const decoder = new TextDecoder();
      let buffer = '';
      let accumulated = '';

      while (true) {
        const { done, value } = await reader.read();
        if (done) break;
        buffer += decoder.decode(value, { stream: true });

        // Parsear eventos SSE separados por línea en blanco
        const events = buffer.split('\n\n');
        buffer = events.pop() || '';

        for (const evt of events) {
          const lines = evt.split('\n');
          let eventName = 'message';
          let data = '';
          for (const line of lines) {
            if (line.startsWith('event:')) eventName = line.slice(6).trim();
            if (line.startsWith('data:')) data += line.slice(5).trim();
          }
          if (!data) continue;

          if (eventName === 'done') {
            store.updateEntry(entryId, { status: 200 });
            continue;
          }
          if (eventName === 'error') {
            try {
              const errObj = JSON.parse(data);
              accumulated += `\n\n[Error: ${errObj.message}]`;
            } catch {
              accumulated += `\n\n[Error: ${data}]`;
            }
            store.updateEntry(entryId, { status: 'error', errorMessage: data });
            continue;
          }

          // chunk normal: { "text": "..." }
          try {
            const parsed = JSON.parse(data);
            const chunk = parsed.text || '';
            accumulated += chunk;
            store.appendChunk(entryId, chunk);
            setMessages((curr) => {
              const updated = [...curr];
              updated[assistantIdx] = { role: 'assistant', content: accumulated };
              return updated;
            });
          } catch {
            // ignore malformed
          }
        }
      }
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Error';
      store.updateEntry(entryId, { status: 'error', errorMessage: msg });
      setMessages((curr) => {
        const updated = [...curr];
        updated[assistantIdx] = { role: 'assistant', content: `[Error: ${msg}]` };
        return updated;
      });
    } finally {
      setStreaming(false);
    }
  };

  return (
    <div className="fixed inset-0 bg-black/40 flex items-end sm:items-center justify-center z-40">
      <div className="bg-white rounded-t-2xl sm:rounded-2xl w-full max-w-2xl mx-0 sm:mx-4 shadow-xl flex flex-col h-[80vh] sm:h-[600px]">
        <header className="flex items-center justify-between px-4 py-3 border-b border-slate-200">
          <div>
            <div className="font-semibold text-sm">💬 Chat sobre esta transacción</div>
            <div className="text-[10px] text-slate-500 mt-0.5 font-mono">{txId.slice(0, 13)}...</div>
          </div>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-700 text-xl">
            ✕
          </button>
        </header>

        <div ref={scrollRef} className="flex-1 overflow-y-auto p-4 space-y-3">
          {messages.length === 0 && (
            <div className="text-center text-slate-500 text-sm py-8">
              Hacé una pregunta sobre esta transacción.
              <br />
              <span className="text-xs">El LLM solo puede hablar sobre esta transacción específica.</span>
            </div>
          )}
          {messages.map((m, i) => (
            <div
              key={i}
              className={`flex ${m.role === 'user' ? 'justify-end' : 'justify-start'}`}
            >
              <div
                className={`max-w-[80%] rounded-2xl px-4 py-2 text-sm whitespace-pre-wrap ${
                  m.role === 'user'
                    ? 'bg-slate-800 text-white'
                    : 'bg-slate-100 text-slate-800'
                }`}
              >
                {m.content || (streaming && i === messages.length - 1 ? '...' : '')}
              </div>
            </div>
          ))}
        </div>

        <div className="border-t border-slate-200 p-3">
          <form
            onSubmit={(e) => {
              e.preventDefault();
              send();
            }}
            className="flex gap-2"
          >
            <input
              type="text"
              value={input}
              onChange={(e) => setInput(e.target.value)}
              placeholder="Escribí tu pregunta..."
              disabled={streaming}
              className="flex-1 border border-slate-300 rounded-full px-4 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-slate-500"
            />
            <button
              type="submit"
              disabled={streaming || !input.trim()}
              className="bg-slate-800 text-white rounded-full px-5 text-sm font-medium hover:bg-slate-900 disabled:opacity-50"
            >
              {streaming ? '...' : 'Enviar'}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
