'use client';

import { useEffect, useState, use } from 'react';
import Link from 'next/link';
import { transactionsApi, llmApi, Transaction, Explanation } from '@/lib/api';
import { TransactionChat } from '@/components/TransactionChat';

export default function TransactionDetailPage({ params }: { params: Promise<{ id: string }> }) {
  const { id } = use(params);
  const [tx, setTx] = useState<Transaction | null>(null);
  const [explanation, setExplanation] = useState<Explanation | null>(null);
  const [chatOpen, setChatOpen] = useState(false);

  useEffect(() => {
    transactionsApi.get(id).then(setTx).catch(console.error);
    // pollea la explicación: el llm-ms tarda 1-2s en generarla tras el evento
    const interval = setInterval(async () => {
      const exp = await llmApi.getExplanation(id);
      if (exp) {
        setExplanation(exp);
        clearInterval(interval);
      }
    }, 1500);
    return () => clearInterval(interval);
  }, [id]);

  if (!tx) {
    return <div className="p-6 text-slate-500">Cargando...</div>;
  }

  return (
    <div className="max-w-3xl mx-auto p-6 space-y-6">
      <header className="flex items-center gap-3">
        <Link href="/" className="text-sm text-slate-500 hover:text-slate-900">
          ← Volver
        </Link>
        <h1 className="text-2xl font-bold">Detalle de transacción</h1>
      </header>

      <div className="bg-white rounded-xl border border-slate-200 p-6 space-y-4">
        <div className="flex items-start justify-between">
          <div>
            <div className="text-xs text-slate-500 font-mono">{tx.id}</div>
            <div className="text-3xl font-bold mt-2">
              {tx.amount} <span className="text-base font-normal text-slate-500">{tx.currency}</span>
            </div>
          </div>
          <StatusBadge status={tx.status} />
        </div>

        <div className="grid grid-cols-2 gap-4 text-sm">
          <div>
            <div className="text-xs text-slate-500">Tipo</div>
            <div className="font-medium">{tx.type}</div>
          </div>
          <div>
            <div className="text-xs text-slate-500">Creada</div>
            <div className="font-medium">{new Date(tx.created_at).toLocaleString('es')}</div>
          </div>
          {tx.from_account_id && (
            <div>
              <div className="text-xs text-slate-500">Desde</div>
              <div className="font-mono text-xs">{tx.from_account_id}</div>
            </div>
          )}
          {tx.to_account_id && (
            <div>
              <div className="text-xs text-slate-500">Hacia</div>
              <div className="font-mono text-xs">{tx.to_account_id}</div>
            </div>
          )}
        </div>

        {tx.status === 'REJECTED' && tx.rejection_msg && (
          <div className="bg-red-50 border border-red-200 rounded p-3 text-sm text-red-800">
            <div className="font-semibold">{tx.rejection_code || 'Rechazada'}</div>
            <div className="text-xs mt-1">{tx.rejection_msg}</div>
          </div>
        )}
      </div>

      {/* Explicación generada por el LLM */}
      <div className="bg-gradient-to-br from-purple-50 to-blue-50 rounded-xl border border-purple-200 p-6">
        <div className="flex items-center gap-2 mb-3">
          <span className="text-xl">🤖</span>
          <h2 className="font-semibold text-purple-900">Explicación generada por el LLM</h2>
        </div>
        {explanation ? (
          <>
            <p className="text-slate-800 leading-relaxed">{explanation.explanation}</p>
            <div className="text-[10px] text-slate-500 mt-3 font-mono">modelo: {explanation.model}</div>
          </>
        ) : (
          <div className="text-sm text-slate-600 italic">
            Generando explicación... (el llm-ms está procesando el evento desde Kafka)
          </div>
        )}
      </div>

      <button
        onClick={() => setChatOpen(true)}
        className="w-full bg-slate-800 hover:bg-slate-900 text-white rounded-lg py-3 text-sm font-medium flex items-center justify-center gap-2"
      >
        💬 Preguntar sobre esta transacción
      </button>

      {chatOpen && <TransactionChat txId={id} onClose={() => setChatOpen(false)} />}
    </div>
  );
}

function StatusBadge({ status }: { status: Transaction['status'] }) {
  const colors: Record<Transaction['status'], string> = {
    PENDING: 'bg-yellow-100 text-yellow-800',
    COMPLETED: 'bg-emerald-100 text-emerald-800',
    REJECTED: 'bg-red-100 text-red-800',
  };
  return (
    <span className={`text-sm font-semibold px-3 py-1 rounded ${colors[status]}`}>{status}</span>
  );
}
