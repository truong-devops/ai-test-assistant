package aibudget

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/maccuatruong/ai-test-assistant/backend/internal/llm"
)

var ErrExceeded = errors.New("document set AI budget exceeded")

type Reservation struct {
	ID             int64
	DocumentSetID  int64
	ReservedTokens int64
	ReservedCost   int64
}

type workflowAttemptKey struct{}

type WorkflowAttempt struct {
	JobID   int64
	UnitID  int64
	Attempt int
}

// WithWorkflowAttempt ties reservations made by an LLM call to the durable
// workflow attempt that caused it. Providers are at-least-once, so this audit
// link is intentionally recorded before the request is sent.
func WithWorkflowAttempt(ctx context.Context, jobID, unitID int64, attempt int) context.Context {
	if jobID <= 0 || unitID <= 0 || attempt <= 0 {
		return ctx
	}
	return context.WithValue(ctx, workflowAttemptKey{}, WorkflowAttempt{
		JobID: jobID, UnitID: unitID, Attempt: attempt})
}

type Status struct {
	UnreconciledReservations int64 `json:"unreconciled_reservations"`
	DocumentSetID            int64 `json:"document_set_id"`
	TokenBudget              int64 `json:"token_budget"`
	UsedTokens               int64 `json:"used_tokens"`
	ReservedTokens           int64 `json:"reserved_tokens"`
	RemainingTokens          int64 `json:"remaining_tokens"`
	CostBudgetMicroUSD       int64 `json:"cost_budget_microusd"`
	UsedCostMicroUSD         int64 `json:"used_cost_microusd"`
	ReservedCostMicroUSD     int64 `json:"reserved_cost_microusd"`
	RemainingCostMicroUSD    int64 `json:"remaining_cost_microusd"`
}

type Controller interface {
	Reserve(context.Context, int64, string, string, llm.Request) (Reservation, error)
	Finalize(context.Context, Reservation, llm.Usage) error
	Release(context.Context, Reservation) error
	Uncertain(context.Context, Reservation) error
}

type Manager struct {
	pool                     *pgxpool.Pool
	inputRate, outputRateUSD float64
	reservationTTL           time.Duration
}

func NewManager(pool *pgxpool.Pool, inputRate, outputRateUSD float64) *Manager {
	return &Manager{pool: pool, inputRate: inputRate, outputRateUSD: outputRateUSD,
		reservationTTL: 2 * time.Hour}
}

func (m *Manager) Reserve(ctx context.Context, setID int64, phase, subject string,
	request llm.Request,
) (Reservation, error) {
	if m == nil || m.pool == nil {
		return Reservation{}, nil
	}
	reservedInput := estimateInputTokens(request)
	reservedOutput := int64(request.MaxOutputTokens)
	if reservedOutput < 1 {
		reservedOutput = 1
	}
	reservedTokens := reservedInput + reservedOutput
	reservedCost := estimateCost(reservedInput, reservedOutput, m.inputRate, m.outputRateUSD)
	tx, err := m.pool.Begin(ctx)
	if err != nil {
		return Reservation{}, err
	}
	defer tx.Rollback(ctx)
	var tokenBudget, costBudget int64
	var status string
	err = tx.QueryRow(ctx, `SELECT ai_token_budget,ai_cost_budget_microusd,status
		FROM document_sets WHERE id=$1 FOR UPDATE`, setID).Scan(&tokenBudget, &costBudget, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Reservation{}, fmt.Errorf("reserve AI budget: document set not found")
	}
	if err != nil {
		return Reservation{}, err
	}
	if status != "ACTIVE" {
		return Reservation{}, fmt.Errorf("%w: document set is %s", ErrExceeded, status)
	}
	// Expiry means the call needs reconciliation, not that the provider charged
	// zero tokens. A killed worker may have sent the request before losing usage.
	// Keep the conservative hold until explicit Finalize/Release resolves it.
	var usedTokens, heldTokens, usedCost, heldCost int64
	err = tx.QueryRow(ctx, `SELECT
		COALESCE(sum(input_tokens+output_tokens) FILTER (WHERE status='COMPLETED'),0),
		COALESCE(sum(reserved_tokens) FILTER (WHERE status='RESERVED'),0),
		COALESCE(sum(actual_cost_microusd) FILTER (WHERE status='COMPLETED'),0),
		COALESCE(sum(reserved_cost_microusd) FILTER (WHERE status='RESERVED'),0)
		FROM document_ai_budget_reservations WHERE document_set_id=$1`, setID).
		Scan(&usedTokens, &heldTokens, &usedCost, &heldCost)
	if err != nil {
		return Reservation{}, err
	}
	if usedTokens+heldTokens+reservedTokens > tokenBudget {
		return Reservation{}, fmt.Errorf("%w: token budget %d, used %d, reserved %d, request %d",
			ErrExceeded, tokenBudget, usedTokens, heldTokens, reservedTokens)
	}
	if costBudget > 0 && usedCost+heldCost+reservedCost > costBudget {
		return Reservation{}, fmt.Errorf("%w: cost budget %d micro-USD, used %d, reserved %d, request %d",
			ErrExceeded, costBudget, usedCost, heldCost, reservedCost)
	}
	var id int64
	workflowAttempt, _ := ctx.Value(workflowAttemptKey{}).(WorkflowAttempt)
	err = tx.QueryRow(ctx, `INSERT INTO document_ai_budget_reservations
		(document_set_id,phase,subject_key,reserved_tokens,reserved_cost_microusd,expires_at,
		 workflow_job_id,workflow_unit_id,workflow_attempt)
		VALUES($1,$2,$3,$4,$5,NOW()+$6::interval,NULLIF($7,0),NULLIF($8,0),NULLIF($9,0))
		RETURNING id`, setID, phase, subject, reservedTokens, reservedCost,
		m.reservationTTL.String(), workflowAttempt.JobID, workflowAttempt.UnitID,
		workflowAttempt.Attempt).Scan(&id)
	if err != nil {
		return Reservation{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return Reservation{}, err
	}
	return Reservation{ID: id, DocumentSetID: setID, ReservedTokens: reservedTokens,
		ReservedCost: reservedCost}, nil
}

func (m *Manager) Finalize(ctx context.Context, reservation Reservation, usage llm.Usage) error {
	if m == nil || m.pool == nil || reservation.ID == 0 {
		return nil
	}
	inputTokens := int64(max(usage.InputTokens, 0))
	outputTokens := int64(max(usage.OutputTokens, 0))
	cost := estimateCost(inputTokens, outputTokens, m.inputRate, m.outputRateUSD)
	result, err := m.pool.Exec(ctx, `UPDATE document_ai_budget_reservations SET
		status='COMPLETED',input_tokens=$2,output_tokens=$3,actual_cost_microusd=$4,
		completed_at=NOW() WHERE id=$1 AND status='RESERVED'`, reservation.ID,
		inputTokens, outputTokens, cost)
	if err != nil {
		return err
	}
	if result.RowsAffected() != 1 {
		return fmt.Errorf("finalize AI budget reservation: reservation is no longer active")
	}
	return nil
}

func (m *Manager) Release(ctx context.Context, reservation Reservation) error {
	if m == nil || m.pool == nil || reservation.ID == 0 {
		return nil
	}
	_, err := m.pool.Exec(ctx, `UPDATE document_ai_budget_reservations SET
		status='RELEASED',completed_at=NOW() WHERE id=$1 AND status='RESERVED'`, reservation.ID)
	return err
}

// Uncertain keeps the full hold and makes it immediately visible for review.
// A provider/network error alone is not a receipt proving zero billable usage.
func (m *Manager) Uncertain(ctx context.Context, reservation Reservation) error {
	if m == nil || m.pool == nil || reservation.ID == 0 {
		return nil
	}
	_, err := m.pool.Exec(ctx, `UPDATE document_ai_budget_reservations SET expires_at=LEAST(expires_at,NOW()) WHERE id=$1 AND status='RESERVED'`, reservation.ID)
	return err
}

func ResolveProviderError(ctx context.Context, budget Controller, reservation Reservation, cause error) {
	if budget == nil {
		return
	}
	// Disabled is a local rejection before dispatch; all other failures are
	// conservative until actual provider usage/non-billing evidence is available.
	if errors.Is(cause, llm.ErrDisabled) {
		_ = budget.Release(ctx, reservation)
		return
	}
	_ = budget.Uncertain(ctx, reservation)
}

func (m *Manager) Status(ctx context.Context, setID int64) (Status, error) {
	var result Status
	result.DocumentSetID = setID
	err := m.pool.QueryRow(ctx, `SELECT s.ai_token_budget,s.ai_cost_budget_microusd,
		COALESCE(sum(r.input_tokens+r.output_tokens) FILTER (WHERE r.status='COMPLETED'),0),
		COALESCE(sum(r.reserved_tokens) FILTER (WHERE r.status='RESERVED'),0),
		COALESCE(sum(r.actual_cost_microusd) FILTER (WHERE r.status='COMPLETED'),0),
		COALESCE(sum(r.reserved_cost_microusd) FILTER (WHERE r.status='RESERVED'),0),
		count(r.id) FILTER (WHERE r.status='RESERVED' AND r.expires_at<=NOW())
		FROM document_sets s LEFT JOIN document_ai_budget_reservations r ON r.document_set_id=s.id
		WHERE s.id=$1 GROUP BY s.id`, setID).Scan(&result.TokenBudget, &result.CostBudgetMicroUSD,
		&result.UsedTokens, &result.ReservedTokens, &result.UsedCostMicroUSD,
		&result.ReservedCostMicroUSD, &result.UnreconciledReservations)
	if errors.Is(err, pgx.ErrNoRows) {
		return Status{}, fmt.Errorf("AI budget status: document set not found")
	}
	if err != nil {
		return Status{}, err
	}
	result.RemainingTokens = max(result.TokenBudget-result.UsedTokens-result.ReservedTokens, 0)
	result.RemainingCostMicroUSD = max(result.CostBudgetMicroUSD-result.UsedCostMicroUSD-result.ReservedCostMicroUSD, 0)
	return result, nil
}

func estimateInputTokens(request llm.Request) int64 {
	schema, _ := json.Marshal(request.Schema)
	bytes := len(request.Instructions) + len(request.Input) + len(schema) + len(request.SchemaName)
	// One token per UTF-8 byte is deliberately conservative for hard reservation.
	return int64(max(bytes, 1))
}

func estimateCost(inputTokens, outputTokens int64, inputRate, outputRate float64) int64 {
	return int64(math.Ceil(float64(inputTokens)*inputRate + float64(outputTokens)*outputRate))
}
