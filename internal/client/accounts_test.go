package client

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func newAccountsTestClient(t *testing.T, handler http.HandlerFunc) *Client {
	t.Helper()
	srv := httptest.NewServer(handler)
	t.Cleanup(srv.Close)

	return NewClient(ClientConfig{
		APIToken:     "test-token",
		BaseURL:      srv.URL,
		RateLimit:    1000,
		MaxRetries:   1,
		RetryMinWait: time.Millisecond,
		RetryMaxWait: time.Millisecond,
		Timeout:      5 * time.Second,
	})
}

const testAccountID = "201205102227VZ6XU"

func writeJSON(t *testing.T, w http.ResponseWriter, status int, body interface{}) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(body); err != nil {
		t.Fatalf("failed to encode response: %v", err)
	}
}

// ---------------------------------------------------------------------------
// Account decoding
// ---------------------------------------------------------------------------

func TestAccountUnmarshalDecodesCanonicalDocument(t *testing.T) {
	raw := `{
		"_id": "201205102227VZ6XU",
		"type": "subcustomer",
		"customer_name": "Example Client",
		"creation_date": 1332535680000,
		"status": "Active",
		"timezone": "-7",
		"nextBillingDate": "2026-09-01",
		"defaultlocations": ["nam", "eur"]
	}`

	var account Account
	if err := json.Unmarshal([]byte(raw), &account); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if account.ID != testAccountID {
		t.Errorf("expected id %q, got %q", testAccountID, account.ID)
	}
	if account.CustomerName != "Example Client" {
		t.Errorf("expected customer_name 'Example Client', got %q", account.CustomerName)
	}
	if account.CreationDate != 1332535680000 {
		t.Errorf("expected creation_date 1332535680000, got %d", account.CreationDate)
	}
	if account.Timezone != "-7" {
		t.Errorf("expected timezone '-7', got %q", account.Timezone)
	}
	if account.NextBillingDate != "2026-09-01" {
		t.Errorf("expected nextBillingDate '2026-09-01', got %q", account.NextBillingDate)
	}
	if len(account.DefaultLocations) != 2 || account.DefaultLocations[0] != "nam" {
		t.Errorf("unexpected defaultlocations: %v", account.DefaultLocations)
	}
}

func TestAccountUnmarshalAcceptsNumericTimezoneAndStringCreationDate(t *testing.T) {
	raw := `{"_id":"A","timezone":-7,"creation_date":"1332535680000","nextBillingDate":null}`

	var account Account
	if err := json.Unmarshal([]byte(raw), &account); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if account.Timezone != "-7" {
		t.Errorf("expected timezone '-7' from a JSON number, got %q", account.Timezone)
	}
	if account.CreationDate != 1332535680000 {
		t.Errorf("expected creation_date 1332535680000 from a string, got %d", account.CreationDate)
	}
	if account.NextBillingDate != "" {
		t.Errorf("expected empty nextBillingDate for null, got %q", account.NextBillingDate)
	}
}

func TestAccountUnmarshalRejectsNonNumericCreationDate(t *testing.T) {
	var account Account
	err := json.Unmarshal([]byte(`{"_id":"A","creation_date":"not-a-date"}`), &account)
	if err == nil {
		t.Fatal("expected an error for a non-numeric creation_date")
	}
	if !strings.Contains(err.Error(), "creation_date") {
		t.Errorf("expected the error to name creation_date, got %v", err)
	}
}

func TestAccountUnmarshalRejectsMalformedJSON(t *testing.T) {
	var account Account
	if err := json.Unmarshal([]byte(`{"_id":`), &account); err == nil {
		t.Fatal("expected an error for malformed JSON")
	}
}

func TestRawToStringIgnoresStructuredValues(t *testing.T) {
	cases := map[string]string{
		`"abc"`:     "abc",
		`  "abc" `:  "abc",
		`5.5`:       "5.5",
		`null`:      "",
		``:          "",
		`{"a":1}`:   "",
		`["a"]`:     "",
		`"unterm`:   "",
		`untyped`:   "",
		`-0.000001`: "-0.000001",
	}
	for input, want := range cases {
		if got := rawToString(json.RawMessage(input)); got != want {
			t.Errorf("rawToString(%q) = %q, want %q", input, got, want)
		}
	}
}

func TestRawToInt64HandlesEmptyAndQuotedEmpty(t *testing.T) {
	for _, input := range []string{``, `null`, `""`} {
		got, err := rawToInt64(json.RawMessage(input))
		if err != nil {
			t.Fatalf("rawToInt64(%q) returned %v", input, err)
		}
		if got != 0 {
			t.Errorf("rawToInt64(%q) = %d, want 0", input, got)
		}
	}
}

// ---------------------------------------------------------------------------
// ListAccounts
// ---------------------------------------------------------------------------

func TestListAccountsReturnsMapKeyedByID(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/accounts" {
			t.Errorf("expected path /accounts, got %s", r.URL.Path)
		}
		parent := 2
		writeJSON(t, w, http.StatusOK, map[string]AccountSummary{
			"PRIMARY": {Parent: &parent, Name: "Primary", Status: "Active", Count: 40},
			"SUB1":    {Name: "Sub One", Status: "Suspend", Count: 3},
		})
	})

	listing, err := c.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listing) != 2 {
		t.Fatalf("expected 2 accounts, got %d", len(listing))
	}
	if listing["PRIMARY"].Parent == nil || *listing["PRIMARY"].Parent != 2 {
		t.Errorf("expected the primary account to report 2 subaccounts")
	}
	if listing["SUB1"].Parent != nil {
		t.Errorf("expected a subaccount to have no parent count")
	}
}

func TestListAccountsWrapsAPIError(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusForbidden, ErrorResponse{Error: "no access"})
	})

	_, err := c.ListAccounts(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "failed to list accounts") {
		t.Errorf("expected a wrapped error, got %v", err)
	}
}

// A client configured with customer_id appends ?customerid= to every request,
// and NodePing then answers GET /accounts with that one account document rather
// than the ID-keyed listing.
func TestListAccountsDecodesSingleAccountDocumentWhenScopedToSubaccount(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.Query().Get("customerid"); got != "SUB1" {
			t.Errorf("customerid = %q, want SUB1", got)
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"_id":              testAccountID,
			"type":             "subcustomer",
			"creation_date":    1332535680000,
			"status":           "Active",
			"customer_name":    "Example Client",
			"sdomain":          "nodeping.com",
			"parent":           "201203232048C76FG",
			"planId":           "free",
			"timezone":         "-7",
			"defaultlocations": []string{"nam"},
			"emailme":          false,
			"customerid":       testAccountID,
		})
	}).WithCustomerID("SUB1")

	listing, err := c.ListAccounts(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(listing) != 1 {
		t.Fatalf("expected 1 account, got %d: %+v", len(listing), listing)
	}
	summary, ok := listing[testAccountID]
	if !ok {
		t.Fatalf("expected the entry to be keyed by _id, got %+v", listing)
	}
	if summary.Name != "Example Client" {
		t.Errorf("Name = %q, want %q", summary.Name, "Example Client")
	}
	if summary.Status != "Active" {
		t.Errorf("Status = %q, want Active", summary.Status)
	}
	// The document's `parent` is the parent account's ID; AccountSummary.Parent
	// is the subaccount count that identifies the primary account. Copying one
	// into the other would report a subaccount as the primary account.
	if summary.Parent != nil {
		t.Errorf("expected no subaccount count on a subaccount document, got %d", *summary.Parent)
	}
}

func TestListAccountsReportsAnUndecodableListingEntry(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"SUB1":"not an account"}`))
	})

	_, err := c.ListAccounts(context.Background())
	if err == nil {
		t.Fatal("expected an error")
	}
	if !strings.Contains(err.Error(), "failed to list accounts") || !strings.Contains(err.Error(), "SUB1") {
		t.Errorf("expected the error to name the failing entry, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// GetAccount
// ---------------------------------------------------------------------------

func TestGetAccountByID(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts/"+testAccountID {
			t.Errorf("expected path /accounts/%s, got %s", testAccountID, r.URL.Path)
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"_id":              testAccountID,
			"type":             "subcustomer",
			"customer_name":    "Example Client",
			"status":           "Active",
			"timezone":         "-7",
			"defaultlocations": []string{"nam"},
		})
	})

	account, err := c.GetAccount(context.Background(), testAccountID)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if account.CustomerName != "Example Client" {
		t.Errorf("unexpected customer_name %q", account.CustomerName)
	}
}

func TestGetAccountByIDMapsNotFound(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusNotFound, ErrorResponse{Error: "not found"})
	})

	_, err := c.GetAccount(context.Background(), testAccountID)
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
	if notFound.ResourceType != "account" {
		t.Errorf("expected resource type 'account', got %q", notFound.ResourceType)
	}
}

func TestGetAccountTreatsEmptyDocumentAsNotFound(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]interface{}{})
	})

	_, err := c.GetAccount(context.Background(), testAccountID)
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError for an empty document, got %v", err)
	}
}

func TestGetAccountRejectsInvalidID(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should reach the server")
	})

	for _, id := range []string{".", "..", "bad\nid"} {
		_, err := c.GetAccount(context.Background(), id)
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) {
			t.Errorf("id %q: expected *ValidationError, got %v", id, err)
		}
	}
}

func TestGetAccountPrimaryResolvesFromListing(t *testing.T) {
	var paths []string
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/accounts" {
			writeJSON(t, w, http.StatusOK, map[string]interface{}{
				"SUB1":    map[string]interface{}{"name": "Sub One", "status": "Active", "count": 1},
				"PRIMARY": map[string]interface{}{"name": "Primary", "status": "Active", "count": 9, "parent": 1},
			})
			return
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"_id":           "PRIMARY",
			"type":          "customer",
			"customer_name": "Primary",
			"status":        "Active",
		})
	})

	account, err := c.GetAccount(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if account.ID != "PRIMARY" {
		t.Errorf("expected the primary account, got %q", account.ID)
	}
	if len(paths) != 2 || paths[1] != "/accounts/PRIMARY" {
		t.Errorf("expected a follow-up GET on the primary account, got %v", paths)
	}
}

func TestGetAccountPrimaryAcceptsSingleDocumentResponse(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts" {
			t.Errorf("unexpected follow-up request to %s", r.URL.Path)
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"_id":           "PRIMARY",
			"customer_name": "Primary",
			"timezone":      "0",
		})
	})

	account, err := c.GetAccount(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if account.ID != "PRIMARY" || account.Timezone != "0" {
		t.Errorf("unexpected account %+v", account)
	}
}

func TestGetAccountPrimaryFallsBackToTheOnlyEntry(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts" {
			writeJSON(t, w, http.StatusOK, map[string]interface{}{
				"ONLY": map[string]interface{}{"name": "Only", "status": "Active"},
			})
			return
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{"_id": "ONLY"})
	})

	account, err := c.GetAccount(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if account.ID != "ONLY" {
		t.Errorf("expected 'ONLY', got %q", account.ID)
	}
}

func TestGetAccountPrimaryIsAmbiguousWithoutParentMarker(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]interface{}{
			"A": map[string]interface{}{"name": "A"},
			"B": map[string]interface{}{"name": "B"},
		})
	})

	_, err := c.GetAccount(context.Background(), "")
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
}

func TestGetAccountPrimaryReportsEmptyListing(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]interface{}{})
	})

	_, err := c.GetAccount(context.Background(), "")
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

func TestGetAccountPrimarySkipsUnparseableEntries(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/accounts" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			// "BROKEN" is a scalar, not an object, so it cannot carry a parent.
			if _, err := w.Write([]byte(`{"BROKEN": 7, "PRIMARY": {"parent": 0}}`)); err != nil {
				t.Fatalf("write failed: %v", err)
			}
			return
		}
		writeJSON(t, w, http.StatusOK, map[string]interface{}{"_id": "PRIMARY"})
	})

	account, err := c.GetAccount(context.Background(), "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if account.ID != "PRIMARY" {
		t.Errorf("expected 'PRIMARY', got %q", account.ID)
	}
}

func TestGetAccountPrimaryWrapsListingError(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusForbidden, ErrorResponse{Error: "denied"})
	})

	_, err := c.GetAccount(context.Background(), "")
	if err == nil || !strings.Contains(err.Error(), "failed to get account") {
		t.Fatalf("expected a wrapped error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// CreateSubaccount
// ---------------------------------------------------------------------------

func validCreateRequest() SubaccountCreateRequest {
	return SubaccountCreateRequest{
		Name:        "Example Client",
		ContactName: "Example Operations",
		Email:       "ops@example.com",
		Timezone:    "-7",
		Location:    "nam",
		EmailMe:     "no",
	}
}

func TestCreateSubaccountReturnsID(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/accounts" {
			t.Errorf("expected path /accounts, got %s", r.URL.Path)
		}

		var body SubaccountCreateRequest
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		if body.ContactName != "Example Operations" || body.Email != "ops@example.com" {
			t.Errorf("create body lost the primary contact: %+v", body)
		}

		writeJSON(t, w, http.StatusOK, map[string]string{"id": testAccountID})
	})

	id, err := c.CreateSubaccount(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != testAccountID {
		t.Errorf("expected %q, got %q", testAccountID, id)
	}
}

func TestCreateSubaccountAcceptsUnderscoreID(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]string{"_id": testAccountID})
	})

	id, err := c.CreateSubaccount(context.Background(), validCreateRequest())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != testAccountID {
		t.Errorf("expected %q, got %q", testAccountID, id)
	}
}

func TestCreateSubaccountErrorsWhenNoIDIsReturned(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusOK, map[string]string{"ok": "true"})
	})

	_, err := c.CreateSubaccount(context.Background(), validCreateRequest())
	if err == nil || !strings.Contains(err.Error(), "no account ID") {
		t.Fatalf("expected a missing-ID error, got %v", err)
	}
}

func TestCreateSubaccountValidatesRequiredFields(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should reach the server")
	})

	fields := map[string]func(*SubaccountCreateRequest){
		"name":        func(r *SubaccountCreateRequest) { r.Name = "  " },
		"contactname": func(r *SubaccountCreateRequest) { r.ContactName = "" },
		"email":       func(r *SubaccountCreateRequest) { r.Email = "" },
		"timezone":    func(r *SubaccountCreateRequest) { r.Timezone = "" },
		"location":    func(r *SubaccountCreateRequest) { r.Location = "" },
	}

	for field, mutate := range fields {
		req := validCreateRequest()
		mutate(&req)

		_, err := c.CreateSubaccount(context.Background(), req)
		var validationErr *ValidationError
		if !errors.As(err, &validationErr) {
			t.Errorf("%s: expected *ValidationError, got %v", field, err)
			continue
		}
		if validationErr.Field != field {
			t.Errorf("expected the error to name %q, got %q", field, validationErr.Field)
		}
	}
}

func TestCreateSubaccountWrapsAPIError(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusBadRequest, ErrorResponse{Error: "bad location"})
	})

	_, err := c.CreateSubaccount(context.Background(), validCreateRequest())
	if err == nil || !strings.Contains(err.Error(), "failed to create subaccount") {
		t.Fatalf("expected a wrapped error, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// UpdateAccount
// ---------------------------------------------------------------------------

func TestUpdateAccountSendsOnlyPopulatedFields(t *testing.T) {
	var body map[string]interface{}
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if r.URL.Path != "/accounts/"+testAccountID {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Fatalf("failed to decode body: %v", err)
		}
		writeJSON(t, w, http.StatusOK, map[string]bool{"ok": true})
	})

	err := c.UpdateAccount(context.Background(), testAccountID, AccountUpdateRequest{Status: "Suspend"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(body) != 1 {
		t.Fatalf("expected only status in the body, got %v", body)
	}
	if body["status"] != "Suspend" {
		t.Errorf("expected status 'Suspend', got %v", body["status"])
	}
}

func TestUpdateAccountWithoutIDTargetsThePrimaryAccount(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/accounts" {
			t.Errorf("expected path /accounts, got %s", r.URL.Path)
		}
		writeJSON(t, w, http.StatusOK, map[string]bool{"ok": true})
	})

	if err := c.UpdateAccount(context.Background(), "", AccountUpdateRequest{Name: "New"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateAccountMapsNotFound(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusNotFound, ErrorResponse{Error: "gone"})
	})

	err := c.UpdateAccount(context.Background(), testAccountID, AccountUpdateRequest{Name: "New"})
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError, got %v", err)
	}
}

func TestUpdateAccountRejectsInvalidID(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should reach the server")
	})

	err := c.UpdateAccount(context.Background(), "..", AccountUpdateRequest{Name: "New"})
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
}

func TestUpdateAccountWrapsAPIError(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusBadRequest, ErrorResponse{Error: "status unsupported on this account"})
	})

	err := c.UpdateAccount(context.Background(), testAccountID, AccountUpdateRequest{Status: "Suspend"})
	if err == nil || !strings.Contains(err.Error(), "failed to update account") {
		t.Fatalf("expected a wrapped error, got %v", err)
	}
}

func TestAccountUpdateRequestIsEmpty(t *testing.T) {
	if !(AccountUpdateRequest{}).IsEmpty() {
		t.Error("a zero request should be empty")
	}
	if (AccountUpdateRequest{EmailMe: "yes"}).IsEmpty() {
		t.Error("a populated request should not be empty")
	}
}

// ---------------------------------------------------------------------------
// DeleteAccount
// ---------------------------------------------------------------------------

func TestDeleteAccountAcceptsQueuedResponse(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		if r.URL.Path != "/accounts/"+testAccountID {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if got := r.URL.Query().Get("customerid"); got != testAccountID {
			t.Errorf("expected customerid=%s, got %q", testAccountID, got)
		}
		writeJSON(t, w, http.StatusOK, AccountDeleteResponse{Success: "Records queued for deletion."})
	})

	if err := c.DeleteAccount(context.Background(), testAccountID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDeleteAccountIsIdempotentOnNotFound(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusNotFound, ErrorResponse{Error: "gone"})
	})

	err := c.DeleteAccount(context.Background(), testAccountID)
	var notFound *NotFoundError
	if !errors.As(err, &notFound) {
		t.Fatalf("expected *NotFoundError so callers can treat it as success, got %v", err)
	}
}

func TestDeleteAccountRefusesEmptyID(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should reach the server")
	})

	err := c.DeleteAccount(context.Background(), "")
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
}

func TestDeleteAccountRefusesTheClientsOwnAccount(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should reach the server")
	}))
	t.Cleanup(srv.Close)

	c := NewClient(ClientConfig{
		APIToken:   "test-token",
		BaseURL:    srv.URL,
		CustomerID: testAccountID,
		RateLimit:  1000,
	})

	err := c.DeleteAccount(context.Background(), testAccountID)
	var validationErr *ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	if !strings.Contains(validationErr.Message, "refusing to delete") {
		t.Errorf("unexpected message %q", validationErr.Message)
	}
}

func TestDeleteAccountWrapsAPIError(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(t, w, http.StatusForbidden, ErrorResponse{Error: "cannot delete your own account"})
	})

	err := c.DeleteAccount(context.Background(), testAccountID)
	if err == nil || !strings.Contains(err.Error(), "failed to delete account") {
		t.Fatalf("expected a wrapped error, got %v", err)
	}
}

func TestAccountRequestsHonorContextCancellation(t *testing.T) {
	c := newAccountsTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("no request should reach the server")
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.ListAccounts(ctx); err == nil {
		t.Error("expected ListAccounts to fail on a cancelled context")
	}
	if _, err := c.GetAccount(ctx, testAccountID); err == nil {
		t.Error("expected GetAccount to fail on a cancelled context")
	}
	if err := c.DeleteAccount(ctx, testAccountID); err == nil {
		t.Error("expected DeleteAccount to fail on a cancelled context")
	}
}

// NodePing returns emailme as a boolean although it is written as "yes"/"no".
func TestAccountUnmarshal_ReadsEmailMeInEveryForm(t *testing.T) {
	for body, want := range map[string]string{
		`{"_id":"A","emailme":false}`: "no",
		`{"_id":"A","emailme":true}`:  "yes",
		`{"_id":"A","emailme":"yes"}`: "yes",
		`{"_id":"A","emailme":"no"}`:  "no",
		`{"_id":"A"}`:                 "",
		`{"_id":"A","emailme":7}`:     "",
	} {
		var a Account
		if err := json.Unmarshal([]byte(body), &a); err != nil {
			t.Fatalf("%s: %v", body, err)
		}
		if a.EmailMe != want {
			t.Errorf("%s: emailme = %q, want %q", body, a.EmailMe, want)
		}
	}
}
