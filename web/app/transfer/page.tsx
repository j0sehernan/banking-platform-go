'use client';

import { useState, useEffect } from 'react';
import { useRouter } from 'next/navigation';
import Link from 'next/link';
import { accountsApi, transactionsApi, Account, Transaction } from '@/lib/api';

export default function TransferPage() {
  const router = useRouter();
  const [accounts, setAccounts] = useState<Account[]>([]);
  const [type, setType] = useState<'TRANSFER' | 'DEPOSIT' | 'WITHDRAW'>('TRANSFER');
  const [from, setFrom] = useState('');
  const [to, setTo] = useState('');
  const [amount, setAmount] = useState('');
  const [currency, setCurrency] = useState('USD');
  const [submitting, setSubmitting] = useState(false);
  const [tx, setTx] = useState<Transaction | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    accountsApi.listAccounts().then((accs) => setAccounts(accs || []));
  }, []);

  // Polling on create: the flow is async (Kafka), the front polls until
  // the status leaves PENDING.
  useEffect(() => {
    if (!tx || tx.status !== 'PENDING') return;
    const id = setInterval(async () => {
      try {
        const updated = await transactionsApi.get(tx.id);
        setTx(updated);
        if (updated.status !== 'PENDING') {
          clearInterval(id);
          setTimeout(() => router.push(`/transactions/${updated.id}`), 800);
        }
      } catch {
        // ignore
      }
    }, 1000);
    return () => clearInterval(id);
  }, [tx, router]);

  const submit = async (e: React.FormEvent) => {
    e.preventDefault();
    setSubmitting(true);
    setError(null);
    setTx(null);
    const idempotencyKey = `idem-${Date.now()}-${Math.random().toString(36).slice(2, 10)}`;
    try {
      let result: Transaction;
      if (type === 'DEPOSIT') {
        result = await transactionsApi.deposit({
          to_account_id: to,
          amount,
          currency,
          idempotency_key: idempotencyKey,
        });
      } else if (type === 'WITHDRAW') {
        result = await transactionsApi.withdraw({
          from_account_id: from,
          amount,
          currency,
          idempotency_key: idempotencyKey,
        });
      } else {
        result = await transactionsApi.transfer({
          from_account_id: from,
          to_account_id: to,
          amount,
          currency,
          idempotency_key: idempotencyKey,
        });
      }
      setTx(result);
    } catch (err) {
      setError(err instanceof Error ? err.message : 'Error');
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <div className="max-w-2xl mx-auto p-6 space-y-6">
      <header className="flex items-center gap-3">
        <Link href="/" className="text-sm text-slate-500 hover:text-slate-900">
          ← Back
        </Link>
        <h1 className="text-2xl font-bold">New transaction</h1>
      </header>

      <form onSubmit={submit} className="bg-white rounded-xl border border-slate-200 p-6 space-y-4">
        <div>
          <label className="text-xs font-medium text-slate-700">Type</label>
          <div className="grid grid-cols-3 gap-2 mt-1">
            {(['TRANSFER', 'DEPOSIT', 'WITHDRAW'] as const).map((t) => (
              <button
                key={t}
                type="button"
                onClick={() => setType(t)}
                className={`py-2 rounded text-sm font-medium ${
                  type === t ? 'bg-slate-800 text-white' : 'bg-slate-100 text-slate-700 hover:bg-slate-200'
                }`}
              >
                {t}
              </button>
            ))}
          </div>
        </div>

        {(type === 'WITHDRAW' || type === 'TRANSFER') && (
          <AccountSelect label="From" value={from} onChange={setFrom} accounts={accounts} />
        )}
        {(type === 'DEPOSIT' || type === 'TRANSFER') && (
          <AccountSelect label="To" value={to} onChange={setTo} accounts={accounts} />
        )}

        <div className="grid grid-cols-2 gap-3">
          <div>
            <label className="text-xs font-medium text-slate-700">Amount</label>
            <input
              type="number"
              step="0.01"
              min="0.01"
              value={amount}
              onChange={(e) => setAmount(e.target.value)}
              required
              className="w-full mt-1 border border-slate-300 rounded px-3 py-2 text-sm"
            />
          </div>
          <div>
            <label className="text-xs font-medium text-slate-700">Currency</label>
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
        </div>

        {error && <div className="text-red-600 text-xs">{error}</div>}

        <button
          type="submit"
          disabled={submitting || !!tx}
          className="w-full bg-slate-800 text-white rounded py-3 text-sm font-medium hover:bg-slate-900 disabled:opacity-50"
        >
          {submitting ? 'Sending...' : `Execute ${type}`}
        </button>

        {tx && (
          <div className="border-t pt-4 mt-4 space-y-2">
            <div className="text-xs text-slate-500">Transaction created</div>
            <div className="font-mono text-xs">{tx.id}</div>
            <div className="flex items-center gap-2">
              <span className="text-sm">Status:</span>
              <StatusBadge status={tx.status} />
              {tx.status === 'PENDING' && (
                <span className="text-xs text-slate-500">waiting for the bus to resolve...</span>
              )}
            </div>
          </div>
        )}
      </form>
    </div>
  );
}

function AccountSelect({
  label,
  value,
  onChange,
  accounts,
}: {
  label: string;
  value: string;
  onChange: (v: string) => void;
  accounts: Account[];
}) {
  return (
    <div>
      <label className="text-xs font-medium text-slate-700">{label}</label>
      <select
        value={value}
        onChange={(e) => onChange(e.target.value)}
        required
        className="w-full mt-1 border border-slate-300 rounded px-3 py-2 text-sm"
      >
        <option value="">Select account...</option>
        {accounts.map((acc) => (
          <option key={acc.id} value={acc.id}>
            {acc.id.slice(0, 8)}... · {acc.balance} {acc.currency}
          </option>
        ))}
      </select>
    </div>
  );
}

function StatusBadge({ status }: { status: Transaction['status'] }) {
  const colors: Record<Transaction['status'], string> = {
    PENDING: 'bg-yellow-100 text-yellow-800',
    COMPLETED: 'bg-emerald-100 text-emerald-800',
    REJECTED: 'bg-red-100 text-red-800',
  };
  return <span className={`text-xs font-semibold px-2 py-1 rounded ${colors[status]}`}>{status}</span>;
}
