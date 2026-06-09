// Package metrics builds and emits the precomputed dashboard artifacts. The
// rollup cube (metrics_daily_v1) is a full recompute from payment_classified_v1;
// the emit step turns it into static JSON. Nothing here is on the collector's
// write path.
package metrics
