//go:build e2e
// +build e2e

// End-to-end test for the full transfer flow.
//
// Assumes the system is up (docker compose up).
// To run it:
//   docker compose up -d
//   go test -tags=e2e ./test/e2e/...
//
// Covers:
//   1. Create a client
//   2. Create 2 accounts
//   3. Deposit balance into one
//   4. Transfer between accounts (happy path)
//   5. Verify final balances
//   6. Verify llm-ms generated the explanation
//   7. Rejected case: transfer with amount > balance
package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

const (
	accountsAPI     = "http://localhost:8081"
	transactionsAPI = "http://localhost:8082"
	llmAPI          = "http://localhost:8083"
)

type clientResp struct {
	ID string `json:"id"`
}

type accountResp struct {
	ID       string `json:"id"`
	Balance  string `json:"balance"`
	Currency string `json:"currency"`
}

type txResp struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	RejectionCode string `json:"rejection_code"`
	RejectionMsg  string `json:"rejection_msg"`
}

type explanationResp struct {
	TxID        string `json:"tx_id"`
	Explanation string `json:"explanation"`
}

func TestE2E_TransferHappyPath(t *testing.T) {
	// 1. Create client
	client := mustCreateClient(t, "Ana Perez", uniqueEmail("ana"))
	t.Logf("created client: %s", client.ID)

	// 2. Create 2 accounts
	accA := mustCreateAccount(t, client.ID, "USD")
	accB := mustCreateAccount(t, client.ID, "USD")
	t.Logf("created accounts: A=%s B=%s", accA.ID, accB.ID)

	// 3. Deposit 1000 into A
	depTx := mustCreateDeposit(t, accA.ID, "1000.00", "USD")
	waitForStatus(t, depTx.ID, "COMPLETED", 15*time.Second)

	// 4. Transfer 300 from A to B
	tx := mustCreateTransfer(t, accA.ID, accB.ID, "300.00", "USD")
	final := waitForStatus(t, tx.ID, "COMPLETED", 15*time.Second)
	if final.Status != "COMPLETED" {
		t.Fatalf("expected COMPLETED, got %s", final.Status)
	}

	// 5. Verify balances
	a := mustGetAccount(t, accA.ID)
	b := mustGetAccount(t, accB.ID)
	if a.Balance != "700.0000" {
		t.Errorf("expected A balance 700.0000, got %s", a.Balance)
	}
	if b.Balance != "300.0000" {
		t.Errorf("expected B balance 300.0000, got %s", b.Balance)
	}

	// 6. Verify llm-ms generated the explanation (with polling)
	exp := waitForExplanation(t, tx.ID, 20*time.Second)
	if exp.Explanation == "" {
		t.Errorf("explanation is empty")
	}
	t.Logf("explanation: %s", exp.Explanation)
}

func TestE2E_TransferRejectedInsufficientFunds(t *testing.T) {
	client := mustCreateClient(t, "Bob", uniqueEmail("bob"))
	accA := mustCreateAccount(t, client.ID, "USD")
	accB := mustCreateAccount(t, client.ID, "USD")

	// transfer more than available (empty account)
	tx := mustCreateTransfer(t, accA.ID, accB.ID, "9999.00", "USD")
	final := waitForStatus(t, tx.ID, "REJECTED", 15*time.Second)

	if final.Status != "REJECTED" {
		t.Fatalf("expected REJECTED, got %s", final.Status)
	}
	if final.RejectionCode != "insufficient_funds" {
		t.Errorf("expected rejection_code=insufficient_funds, got %s", final.RejectionCode)
	}

	// the explanation should be generated for rejections too
	exp := waitForExplanation(t, tx.ID, 20*time.Second)
	if exp.Explanation == "" {
		t.Errorf("expected explanation for rejection")
	}
	t.Logf("rejection explanation: %s", exp.Explanation)
}

// ===== helpers =====

func uniqueEmail(prefix string) string {
	return fmt.Sprintf("%s-%d@test.local", prefix, time.Now().UnixNano())
}

func mustCreateClient(t *testing.T, name, email string) clientResp {
	t.Helper()
	body := mustJSON(map[string]string{"name": name, "email": email})
	var c clientResp
	mustPost(t, accountsAPI+"/clients", body, &c)
	return c
}

func mustCreateAccount(t *testing.T, clientID, currency string) accountResp {
	t.Helper()
	body := mustJSON(map[string]string{"client_id": clientID, "currency": currency})
	var a accountResp
	mustPost(t, accountsAPI+"/accounts", body, &a)
	return a
}

func mustCreateDeposit(t *testing.T, toID, amount, currency string) txResp {
	t.Helper()
	body := mustJSON(map[string]string{
		"to_account_id":   toID,
		"amount":          amount,
		"currency":        currency,
		"idempotency_key": fmt.Sprintf("dep-%d", time.Now().UnixNano()),
	})
	var tx txResp
	mustPost(t, transactionsAPI+"/transactions/deposit", body, &tx)
	return tx
}

func mustCreateTransfer(t *testing.T, fromID, toID, amount, currency string) txResp {
	t.Helper()
	body := mustJSON(map[string]string{
		"from_account_id": fromID,
		"to_account_id":   toID,
		"amount":          amount,
		"currency":        currency,
		"idempotency_key": fmt.Sprintf("tr-%d", time.Now().UnixNano()),
	})
	var tx txResp
	mustPost(t, transactionsAPI+"/transactions/transfer", body, &tx)
	return tx
}

func mustGetAccount(t *testing.T, id string) accountResp {
	t.Helper()
	var a accountResp
	mustGet(t, accountsAPI+"/accounts/"+id, &a)
	return a
}

func waitForStatus(t *testing.T, txID, expected string, timeout time.Duration) txResp {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		var tx txResp
		mustGet(t, transactionsAPI+"/transactions/"+txID, &tx)
		if tx.Status == expected || (tx.Status != "PENDING" && expected != "PENDING") {
			return tx
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("transaction %s did not reach status %s within %v", txID, expected, timeout)
	return txResp{}
}

func waitForExplanation(t *testing.T, txID string, timeout time.Duration) explanationResp {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		res, err := http.Get(llmAPI + "/transactions/" + txID + "/explanation")
		if err == nil && res.StatusCode == 200 {
			var e explanationResp
			_ = json.NewDecoder(res.Body).Decode(&e)
			res.Body.Close()
			return e
		}
		if res != nil {
			res.Body.Close()
		}
		time.Sleep(500 * time.Millisecond)
	}
	t.Fatalf("explanation for %s not generated within %v", txID, timeout)
	return explanationResp{}
}

func mustPost(t *testing.T, url string, body []byte, dst any) {
	t.Helper()
	res, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		respBody, _ := io.ReadAll(res.Body)
		t.Fatalf("POST %s returned %d: %s", url, res.StatusCode, string(respBody))
	}
	if dst != nil {
		if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
}

func mustGet(t *testing.T, url string, dst any) {
	t.Helper()
	res, err := http.Get(url)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	defer res.Body.Close()
	if res.StatusCode >= 300 {
		t.Fatalf("GET %s returned %d", url, res.StatusCode)
	}
	if dst != nil {
		if err := json.NewDecoder(res.Body).Decode(dst); err != nil {
			t.Fatalf("decode response: %v", err)
		}
	}
}

func mustJSON(v any) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
