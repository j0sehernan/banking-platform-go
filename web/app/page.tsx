'use client';

import { useEffect, useState } from 'react';
import Link from 'next/link';
import { accountsApi, transactionsApi, Account, Transaction } from '@/lib/api';

export default function HomePage() {
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [transactions, setTransactions] = useState<Transaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [showNewClient, setShowNewClient] = useState(false);
  const [showNewAccount, setShowNewAccount] = useState(false);

  const refresh = async () => {
    try {
      const [accs, txs] = await Promise.all([
        accountsApi.listAccounts(),
        transactionsApi.list(),
      ]);
      setAccounts(accs || []);
      setTransactions(txs || []);
    } catch (err) {
      console.error('refresh error', err);
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    refresh();
  }, []);

  return (
    <div className="max-w-5xl mx-auto p-6 space-y-6">
      <header className="flex items-center justify-between">
        <div>
          <h1 className="text-2xl font-bold">🏦 Banking Platform</h1>
          <p className="text-sm text-slate-600 mt-1">
            Demo event-driven con Go + Kafka + Claude · revisá el panel derecho para ver los requests
          </p>
        </div>
        <div className="flex gap-2">
          <button
            onClick={() => setShowNewClient(true)}
            className="px-4 py-2 bg-blue-600 text-white rounded-lg hover:bg-blue-700 text-sm font-medium"
          >
            + Cliente
          </button>
          <button
            onClick={() => setShowNewAccount(true)}
            className="px-4 py-2 bg-emerald-600 text-white rounded-lg hover:bg-emerald-700 text-sm font-medium"
          >
            + Cuenta
          </button>
          <Link
            href="/transfer"
            className="px-4 py-2 bg-slate-800 text-white rounded-lg hover:bg-slate-900 text-sm font-medium"
          >
            Transferir →
          </Link>
        </div>
      </header>

      <section>
        <h2 className="text-lg font-semibold mb-3">Cuentas</h2>
        {loading ? (
          <div className="text-slate-500 text-sm">Cargando...</div>
        ) : accounts.length === 0 ? (
          <div className="bg-white rounded-lg border border-slate-200 p-6 text-center text-slate-500 text-sm">
            No hay cuentas todavía. Creá un cliente y luego una cuenta.
          </div>
        ) : (
          <div className="grid grid-cols-1 md:grid-cols-2 gap-3">
            {accounts.map((acc) => (
              <div key={acc.id} className="bg-white rounded-lg border border-slate-200 p-4">
                <div className="flex justify-between items-start">
                  <div>
                    <div className="text-xs text-slate-500 font-mono">{acc.id.slice(0, 8)}...</div>
                    <div className="text-2xl font-bold mt-1">
                      {acc.balance} <span className="text-sm font-normal text-slate-500">{acc.currency}</span>
                    </div>
                  </div>
                  <div className="text-xs text-slate-400">cliente: {acc.client_id.slice(0, 8)}...</div>
                </div>
              </div>
            ))}
          </div>
        )}
      </section>

      <section>
        <h2 className="text-lg font-semibold mb-3">Transacciones recientes</h2>
        {transactions.length === 0 ? (
          <div className="bg-white rounded-lg border border-slate-200 p-6 text-center text-slate-500 text-sm">
            Aún no hay transacciones.
          </div>
        ) : (
          <div className="bg-white rounded-lg border border-slate-200 divide-y divide-slate-100">
            {transactions.map((tx) => (
              <Link
                key={tx.id}
                href={`/transactions/${tx.id}`}
                className="flex items-center justify-between p-3 hover:bg-slate-50 transition"
              >
                <div className="flex items-center gap-3">
                  <StatusBadge status={tx.status} />
                  <div>
                    <div className="text-sm font-medium">{tx.type}</div>
                    <div className="text-xs text-slate-500 font-mono">{tx.id.slice(0, 13)}...</div>
                  </div>
                </div>
                <div className="text-right">
                  <div className="text-sm font-semibold">
                    {tx.amount} {tx.currency}
                  </div>
                  <div className="text-xs text-slate-400">{new Date(tx.created_at).toLocaleString('es')}</div>
                </div>
              </Link>
            ))}
          </div>
        )}
      </section>

      {showNewClient && <NewClientModal onClose={() => setShowNewClient(false)} onCreated={refresh} />}
      {showNewAccount && (
        <NewAccountModal onClose={() => setShowNewAccount(false)} onCreated={refresh} />
      )}
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
    <span className={`text-[10px] font-semibold px-2 py-0.5 rounded ${colors[status]}`}>{status}</span>
  );
}

function NewClientModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [name, setName] = useState('');
  const [email, setEmail] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await accountsApi.createClient({ name, email });
      onCreated();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Error');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal onClose={onClose} title="Nuevo cliente">
      <form onSubmit={submit} className="space-y-3">
        <Input label="Nombre" value={name} onChange={setName} required />
        <Input label="Email" value={email} onChange={setEmail} type="email" required />
        {error && <div className="text-red-600 text-xs">{error}</div>}
        <button
          type="submit"
          disabled={submitting}
          className="w-full bg-blue-600 text-white rounded py-2 text-sm font-medium hover:bg-blue-700 disabled:opacity-50"
        >
          {submitting ? 'Creando...' : 'Crear cliente'}
        </button>
      </form>
    </Modal>
  );
}

function NewAccountModal({ onClose, onCreated }: { onClose: () => void; onCreated: () => void }) {
  const [clientID, setClientID] = useState('');
  const [currency, setCurrency] = useState('USD');
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    try {
      await accountsApi.createAccount({ client_id: clientID, currency });
      onCreated();
      onClose();
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Error');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Modal onClose={onClose} title="Nueva cuenta">
      <form onSubmit={submit} className="space-y-3">
        <Input label="Client ID (UUID)" value={clientID} onChange={setClientID} required />
        <div>
          <label className="text-xs font-medium text-slate-700">Moneda</label>
          <select
            value={currency}
            onChange={(e) => setCurrency(e.target.value)}
            className="w-full mt-1 border border-slate-300 rounded px-3 py-2 text-sm"
          >
            <option>USD</option>
            <option>EUR</option>
            <option>ARS</option>
          </select>
        </div>
        {error && <div className="text-red-600 text-xs">{error}</div>}
        <button
          type="submit"
          disabled={submitting}
          className="w-full bg-emerald-600 text-white rounded py-2 text-sm font-medium hover:bg-emerald-700 disabled:opacity-50"
        >
          {submitting ? 'Creando...' : 'Crear cuenta'}
        </button>
      </form>
    </Modal>
  );
}

function Modal({ onClose, title, children }: { onClose: () => void; title: string; children: React.ReactNode }) {
  return (
    <div className="fixed inset-0 bg-black/40 flex items-center justify-center z-50" onClick={onClose}>
      <div
        className="bg-white rounded-xl p-6 w-full max-w-md mx-4 shadow-xl"
        onClick={(e) => e.stopPropagation()}
      >
        <div className="flex justify-between items-center mb-4">
          <h3 className="font-semibold text-lg">{title}</h3>
          <button onClick={onClose} className="text-slate-400 hover:text-slate-700">
            ✕
          </button>
        </div>
        {children}
      </div>
    </div>
  );
}

function Input({
  label,
  value,
  onChange,
  type = 'text',
  required,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  type?: string;
  required?: boolean;
}) {
  return (
    <div>
      <label className="text-xs font-medium text-slate-700">{label}</label>
      <input
        type={type}
        value={value}
        onChange={(e) => onChange(e.target.value)}
        required={required}
        className="w-full mt-1 border border-slate-300 rounded px-3 py-2 text-sm focus:outline-none focus:ring-2 focus:ring-blue-500"
      />
    </div>
  );
}
