// Hand-built fixtures whose buckets reconcile to concentration totals so the
// integrity checks pass. Numbers are illustrative, not from production.
export const payeeDoc = {
  methodology_version: 1,
  generated_at: "2026-06-18T14:41:39Z",
  data_through_day: "2026-06-06",
  data: {
    role: "payee",
    windows: {
      all: {
        leaderboard: [
          // service: many payers, many amounts
          { address: "0xservice000000000000000000000000000000aaaa", volume_usdc: "1000000.00",
            txn_count: 50000, distinct_counterparties: 7000, distinct_amounts: 2000,
            known_volume_usdc: "0.0", first_seen: "2026-05-08 00:00:00+00", last_seen: "2026-06-06 00:00:00+00" },
          // otc: few txns, huge avg
          { address: "0xotc0000000000000000000000000000000000bbbb", volume_usdc: "10000000.00",
            txn_count: 2, distinct_counterparties: 1, distinct_amounts: 2,
            known_volume_usdc: "0.0", first_seen: "2026-05-24 07:01:31+00", last_seen: "2026-05-24 07:02:47+00" },
          // sink: huge txns, tiny avg, few amounts
          { address: "0xsink0000000000000000000000000000000000cccc", volume_usdc: "200.00",
            txn_count: 2000000, distinct_counterparties: 5, distinct_amounts: 3,
            known_volume_usdc: "0.0", first_seen: "2026-05-01 00:00:00+00", last_seen: "2026-06-06 00:00:00+00" },
          // fleet: high txns, one amount, not dust
          { address: "0xfleet000000000000000000000000000000000dddd", volume_usdc: "500000.00",
            txn_count: 1000000, distinct_counterparties: 90, distinct_amounts: 1,
            known_volume_usdc: "0.0", first_seen: "2026-05-01 00:00:00+00", last_seen: "2026-06-06 00:00:00+00" },
          // tail: small, ambiguous
          { address: "0xtail0000000000000000000000000000000000eeee", volume_usdc: "12.00",
            txn_count: 4, distinct_counterparties: 3, distinct_amounts: 2,
            known_volume_usdc: "0.0", first_seen: "2026-06-01 00:00:00+00", last_seen: "2026-06-02 00:00:00+00" },
        ],
        // entity_count sum = 5, txn_sum = 3050007, volume_sum = 11500224.00
        buckets: [
          { bucket: "1", entity_count: 1, txn_sum: 1, volume_sum: "12.00" },
          { bucket: "2-10", entity_count: 2, txn_sum: 6, volume_sum: "10000012.00" },
          { bucket: "11-100", entity_count: 0, txn_sum: 0, volume_sum: "0.00" },
          { bucket: "101-1k", entity_count: 0, txn_sum: 0, volume_sum: "0.00" },
          { bucket: "1k-100k", entity_count: 1, txn_sum: 50000, volume_sum: "1000000.00" },
          { bucket: "100k+", entity_count: 1, txn_sum: 3000000, volume_sum: "500200.00" },
        ],
        concentration: {
          total_entities: 5, total_txns: 3050007, total_volume: "11500224.00",
          top10_volume: "11500224.00", top10_txns: 3050007, top100_volume: "11500224.00",
        },
      },
    },
  },
};
// NOTE: txn_sum across buckets = 1+6+0+0+50000+3000000 = 3050007; matches total_txns.
// volume_sum = 12 + 10000012 + 0 + 0 + 1000000 + 500200 = 11500224.00; matches total_volume.
// entity_count = 1+2+0+0+1+1 = 5; matches total_entities.

export const sparseDoc = {
  methodology_version: 1,
  generated_at: "2026-06-18T14:41:39Z",
  data_through_day: "2026-06-06",
  data: {
    role: "payee",
    windows: {
      all: {
        leaderboard: [],
        buckets: [],
        concentration: { total_entities: 0, total_txns: 0, total_volume: "0.00",
          top10_volume: "0.00", top10_txns: 0, top100_volume: "0.00" },
      },
    },
  },
};

export const economyDoc = {
  methodology_version: 1,
  generated_at: "2026-06-18T14:41:39Z",
  data_through_day: "2026-06-06",
  data: {
    concentration: {
      windows: {
        all: {
          payee: { total_entities: 5, total_txns: 3050007, total_volume: "11500224.00",
            top10_volume: "11500224.00", top10_txns: 3050007, top100_volume: "11500224.00" },
          payer: { total_entities: 5, total_txns: 3050007, total_volume: "11500224.00",
            top10_volume: "11500224.00", top10_txns: 3050007, top100_volume: "11500224.00" },
        },
      },
    },
  },
};
