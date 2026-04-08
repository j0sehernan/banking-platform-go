// Wrapper over fetch that captures every call into the activity store.
// Every front call to the microservices goes through here so the
// activity panel can show the details in real time.
import { useActivityStore } from './activityStore';

const ACCOUNTS = process.env.NEXT_PUBLIC_ACCOUNTS_API || 'http://localhost:8081';
const TRANSACTIONS = process.env.NEXT_PUBLIC_TRANSACTIONS_API || 'http://localhost:8082';
const LLM = process.env.NEXT_PUBLIC_LLM_API || 'http://localhost:8083';

export const API = {
  accounts: ACCOUNTS,
  transactions: TRANSACTIONS,
  llm: LLM,
};

export type FetchOptions = RequestInit & {
  body?: BodyInit | null;
};

// trackedFetch wraps fetch() to record activity.
// For JSON responses, it parses the body. For errors, it captures the message.
export async function trackedFetch(url: string, options: FetchOptions = {}): Promise<Response> {
  const start = Date.now();
  const method = (options.method || 'GET').toUpperCase();
  const store = useActivityStore.getState();

  let parsedRequest: unknown = undefined;
  if (options.body && typeof options.body === 'string') {
    try {
      parsedRequest = JSON.parse(options.body);
    } catch {
      parsedRequest = options.body;
    }
  }

  const entryId = store.addEntry({
    method,
    url: url.replace(/^https?:\/\/[^/]+/, ''), // path-only for the panel
    status: 'pending',
    requestBody: parsedRequest,
  });

  try {
    const res = await fetch(url, {
      ...options,
      headers: {
        'Content-Type': 'application/json',
        ...(options.headers || {}),
      },
    });

    let parsedResponse: unknown = undefined;
    const cloned = res.clone();
    try {
      parsedResponse = await cloned.json();
    } catch {
      parsedResponse = undefined;
    }

    store.updateEntry(entryId, {
      status: res.status,
      durationMs: Date.now() - start,
      responseBody: parsedResponse,
    });

    return res;
  } catch (err) {
    store.updateEntry(entryId, {
      status: 'error',
      durationMs: Date.now() - start,
      errorMessage: err instanceof Error ? err.message : String(err),
    });
    throw err;
  }
}

// ===== typed helpers for each API =====

export type Client = {
  id: string;
  name: string;
  email: string;
  created_at: string;
};

export type Account = {
  id: string;
  client_id: string;
  currency: string;
  balance: string;
  created_at: string;
  updated_at: string;
};

export type Transaction = {
  id: string;
  type: 'DEPOSIT' | 'WITHDRAW' | 'TRANSFER';
  from_account_id?: string;
  to_account_id?: string;
  amount: string;
  currency: string;
  status: 'PENDING' | 'COMPLETED' | 'REJECTED';
  rejection_code?: string;
  rejection_msg?: string;
  created_at: string;
  updated_at: string;
};

export type Explanation = {
  tx_id: string;
  explanation: string;
  model: string;
  generated_at: string;
};

export const accountsApi = {
  createClient: async (data: { name: string; email: string }): Promise<Client> => {
    const res = await trackedFetch(`${ACCOUNTS}/clients`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
    if (!res.ok) throw await asError(res);
    return res.json();
  },

  listClients: async (): Promise<Client[]> => {
    const res = await trackedFetch(`${ACCOUNTS}/clients`);
    if (!res.ok) throw await asError(res);
    return res.json();
  },

  createAccount: async (data: { client_id: string; currency: string }): Promise<Account> => {
    const res = await trackedFetch(`${ACCOUNTS}/accounts`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
    if (!res.ok) throw await asError(res);
    return res.json();
  },

  listAccounts: async (): Promise<Account[]> => {
    const res = await trackedFetch(`${ACCOUNTS}/accounts`);
    if (!res.ok) throw await asError(res);
    return res.json();
  },

  getAccount: async (id: string): Promise<Account> => {
    const res = await trackedFetch(`${ACCOUNTS}/accounts/${id}`);
    if (!res.ok) throw await asError(res);
    return res.json();
  },
};

export const transactionsApi = {
  deposit: async (data: { to_account_id: string; amount: string; currency: string; idempotency_key: string }): Promise<Transaction> => {
    const res = await trackedFetch(`${TRANSACTIONS}/transactions/deposit`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
    if (!res.ok) throw await asError(res);
    return res.json();
  },

  withdraw: async (data: { from_account_id: string; amount: string; currency: string; idempotency_key: string }): Promise<Transaction> => {
    const res = await trackedFetch(`${TRANSACTIONS}/transactions/withdraw`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
    if (!res.ok) throw await asError(res);
    return res.json();
  },

  transfer: async (data: { from_account_id: string; to_account_id: string; amount: string; currency: string; idempotency_key: string }): Promise<Transaction> => {
    const res = await trackedFetch(`${TRANSACTIONS}/transactions/transfer`, {
      method: 'POST',
      body: JSON.stringify(data),
    });
    if (!res.ok) throw await asError(res);
    return res.json();
  },

  get: async (id: string): Promise<Transaction> => {
    const res = await trackedFetch(`${TRANSACTIONS}/transactions/${id}`);
    if (!res.ok) throw await asError(res);
    return res.json();
  },

  list: async (): Promise<Transaction[]> => {
    const res = await trackedFetch(`${TRANSACTIONS}/transactions`);
    if (!res.ok) throw await asError(res);
    return res.json();
  },
};

export const llmApi = {
  getExplanation: async (txId: string): Promise<Explanation | null> => {
    const res = await trackedFetch(`${LLM}/transactions/${txId}/explanation`);
    if (res.status === 404) return null;
    if (!res.ok) throw await asError(res);
    return res.json();
  },
};

async function asError(res: Response): Promise<Error> {
  try {
    const body = await res.json();
    if (body?.error?.message) {
      return new Error(body.error.message);
    }
  } catch {
    // ignore
  }
  return new Error(`HTTP ${res.status}`);
}
