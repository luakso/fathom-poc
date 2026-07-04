package anatomy

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// PgEntity serves all per-entity reads from the Plan A aggregate tables.
// One struct implements EntityProvider, NeighborProvider, ActivityProvider,
// and ListProvider (Tasks 4-7); each interface stays small, the SQL shares
// one home per concern file.
type PgEntity struct{ pool *pgxpool.Pool }

// NewPgEntity constructs a PgEntity backed by the given connection pool.
func NewPgEntity(pool *pgxpool.Pool) *PgEntity { return &PgEntity{pool: pool} }

// dayRow is one (role, day, lens) slice from entity_day_v1.
type dayRow struct {
	role  string
	day   string
	known bool
	txns  int64
	vol   string
}

// readDayRows fetches every day row for an address (bounded: <= roles x days x 2).
func (p *PgEntity) readDayRows(ctx context.Context, chain, addr string) ([]dayRow, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT role, day::text, facilitator_known, txn_count, volume_usdc::text
		FROM entity_day_v1
		WHERE chain = $1 AND address = $2
		ORDER BY role, day, facilitator_known`, chain, addr)
	if err != nil {
		return nil, fmt.Errorf("entity day rows: %w", err)
	}
	defer rows.Close()
	var out []dayRow
	for rows.Next() {
		var r dayRow
		if err := rows.Scan(&r.role, &r.day, &r.known, &r.txns, &r.vol); err != nil {
			return nil, fmt.Errorf("scan day row: %w", err)
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// Entity implements EntityProvider.
func (p *PgEntity) Entity(ctx context.Context, chain, address string) (Entity, error) {
	days, err := p.readDayRows(ctx, chain, address)
	if err != nil {
		return Entity{}, err
	}
	signals, err := p.readSignals(ctx, chain, address)
	if err != nil {
		return Entity{}, err
	}
	if len(days) == 0 && len(signals) == 0 {
		return Entity{}, ErrNotFound
	}

	e := Entity{
		Chain:     chain,
		Address:   address,
		Signals:   signals,
		Summaries: map[string]map[string]LensSummary{},
	}
	if err := p.resolveLabel(ctx, chain, address, &e); err != nil {
		return Entity{}, err
	}

	// Volume sums must stay exact -> summed in SQL per (role, lens); the day
	// rows drive counts and day boundaries only.
	type key struct {
		role string
		lens string
	}
	sums := map[key]*LensSummary{}
	get := func(role, lens string) *LensSummary {
		k := key{role, lens}
		if sums[k] == nil {
			sums[k] = &LensSummary{VolumeUSDC: "0"}
		}
		return sums[k]
	}
	seenDay := map[key]map[string]bool{}
	markDay := func(role, lens, day string) {
		k := key{role, lens}
		if seenDay[k] == nil {
			seenDay[k] = map[string]bool{}
		}
		if !seenDay[k][day] {
			seenDay[k][day] = true
			s := get(role, lens)
			s.ActiveDays++
			if s.FirstDay == "" || day < s.FirstDay {
				s.FirstDay = day
			}
			if day > s.LastDay {
				s.LastDay = day
			}
		}
	}
	for _, d := range days {
		all := get(d.role, "all")
		all.TxnCount += d.txns
		markDay(d.role, "all", d.day)
		if d.known {
			kn := get(d.role, "known")
			kn.TxnCount += d.txns
			markDay(d.role, "known", d.day)
		}
	}
	if err := p.fillVolumes(ctx, chain, address, get); err != nil {
		return Entity{}, err
	}
	if err := p.fillCounterpartyCounts(ctx, chain, address, get); err != nil {
		return Entity{}, err
	}

	roles := map[string]bool{}
	for k, s := range sums {
		if e.Summaries[k.role] == nil {
			e.Summaries[k.role] = map[string]LensSummary{}
		}
		e.Summaries[k.role][k.lens] = *s
		roles[k.role] = true
	}
	for r := range roles {
		e.Roles = append(e.Roles, r)
	}
	sort.Strings(e.Roles)
	return e, nil
}

// fillVolumes sums volume per (role, lens) in SQL to preserve decimal exactness.
func (p *PgEntity) fillVolumes(ctx context.Context, chain, addr string, get func(role, lens string) *LensSummary) error {
	rows, err := p.pool.Query(ctx, `
		SELECT role, lens, sum(volume_usdc)::text FROM (
		    SELECT role, 'all' AS lens, volume_usdc FROM entity_day_v1 WHERE chain=$1 AND address=$2
		    UNION ALL
		    SELECT role, 'known', volume_usdc FROM entity_day_v1 WHERE chain=$1 AND address=$2 AND facilitator_known
		) t GROUP BY role, lens`, chain, addr)
	if err != nil {
		return fmt.Errorf("entity volumes: %w", err)
	}
	defer rows.Close()
	for rows.Next() {
		var role, lens, vol string
		if err := rows.Scan(&role, &lens, &vol); err != nil {
			return fmt.Errorf("scan entity volume: %w", err)
		}
		get(role, lens).VolumeUSDC = vol
	}
	return rows.Err()
}

// fillCounterpartyCounts computes exact distinct counterparties per role+lens.
func (p *PgEntity) fillCounterpartyCounts(ctx context.Context, chain, addr string, get func(role, lens string) *LensSummary) error {
	row := p.pool.QueryRow(ctx, `
		SELECT
		  (SELECT count(DISTINCT payee) FROM entity_edge_v1 WHERE chain=$1 AND payer=$2 AND facilitator_known),
		  (SELECT count(DISTINCT payee) FROM entity_edge_v1 WHERE chain=$1 AND payer=$2),
		  (SELECT count(DISTINCT payer) FROM entity_edge_v1 WHERE chain=$1 AND payee=$2 AND facilitator_known),
		  (SELECT count(DISTINCT payer) FROM entity_edge_v1 WHERE chain=$1 AND payee=$2),
		  (SELECT count(DISTINCT counterparty) FROM facilitator_edge_v1 WHERE chain=$1 AND facilitator=$2 AND facilitator_known),
		  (SELECT count(DISTINCT counterparty) FROM facilitator_edge_v1 WHERE chain=$1 AND facilitator=$2)`,
		chain, addr)
	var payerK, payerA, payeeK, payeeA, facK, facA int64
	if err := row.Scan(&payerK, &payerA, &payeeK, &payeeA, &facK, &facA); err != nil {
		return fmt.Errorf("entity counterparty counts: %w", err)
	}
	set := func(role string, k, a int64) {
		if k > 0 {
			get(role, "known").DistinctCounterparties = k
		}
		if a > 0 {
			get(role, "all").DistinctCounterparties = a
		}
	}
	set("payer", payerK, payerA)
	set("payee", payeeK, payeeA)
	set("facilitator", facK, facA)
	return nil
}

// readSignals lists raw provenance rows; resolveLabel picks the display label.
func (p *PgEntity) readSignals(ctx context.Context, chain, addr string) ([]IdentitySignal, error) {
	rows, err := p.pool.Query(ctx, `
		SELECT source, kind, value, COALESCE(url,''), fetched_at::text
		FROM entity_signal WHERE chain = $1 AND address = $2
		ORDER BY source, kind`, chain, addr)
	if err != nil {
		return nil, fmt.Errorf("entity signals: %w", err)
	}
	defer rows.Close()
	var out []IdentitySignal
	for rows.Next() {
		var s IdentitySignal
		if err := rows.Scan(&s.Source, &s.Kind, &s.Value, &s.URL, &s.FetchedAt); err != nil {
			return nil, fmt.Errorf("scan signal: %w", err)
		}
		out = append(out, s)
	}
	return out, rows.Err()
}

func (p *PgEntity) resolveLabel(ctx context.Context, chain, addr string, e *Entity) error {
	err := p.pool.QueryRow(ctx, `
		SELECT label, source FROM entity_identity_v1 WHERE chain = $1 AND address = $2`,
		chain, addr).Scan(&e.Label, &e.LabelSource)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("resolve identity: %w", err)
	}
	return nil
}
