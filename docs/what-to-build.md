# What to Build — The Agent Economy Index

## What you're actually building

The product is a **weekly published index of the agent economy**, plus the data infrastructure that makes it credible. Not x402-only. Not organic-vs-synthetic. The frame is: *measure the flow of money through agent-mediated payments, across all protocols, and become the canonical source.*

The system has three layers and you build them in this order:

1. **The collector** — pulls data from the world into your store
2. **The warehouse** — stores it in a way that makes weekly publishing easy and methodology defensible
3. **The publisher** — turns the warehouse into the weekly artifact

Most of the complexity is in layer 1. Layers 2 and 3 are deliberately boring.

## Layer 1: The collector

The job is to observe agent-payment activity across the protocols that matter. For v1, scope ruthlessly. Start with exactly these sources, in this order:

**Source 1: x402 on Base.** Public on-chain data, well-documented, the most volume right now. Index the facilitator contracts and known x402 settlement addresses. Output: every x402 payment with timestamp, payer, payee (service), amount, asset.

**Source 2: x402 on Solana.** Same shape, different chain. Adds protocol diversity to the numbers, which matters for credibility.

**Source 3: A registry of known 402 endpoints.** Maintained list of HTTP endpoints that speak some flavor of 402. Probe them periodically — not their traffic, but their existence, their pricing, their declared payment methods. This gives the *supply side* of the index (how many services exist, what they cost, what rails they accept). Cheap to build, distinctive data, no other index has this.

That's it for v1. Three sources, two on-chain and one supply-side. Do not add L402/Lightning, Stripe-agent flows, or proprietary integrations yet. Each of those is its own project and they can come in v2/v3.

**Technical shape in Go:** one service per source, each writing into a shared Postgres database. Each collector is independent — if one breaks, the others keep running. Each one has a `--from-block X` or `--since T` flag for backfill and replay. Postgres because SQL is needed for analysis and ad-hoc questions, not because the volume requires it. All of this runs on a single $20/month VPS for the foreseeable future.

The on-chain collectors are roughly: connect to an RPC endpoint, subscribe to relevant events or poll blocks, decode the events that matter, upsert into Postgres, track the cursor. Mature Go libraries exist for both Base (go-ethereum) and Solana (solana-go). The hard parts aren't the indexing — they're the decisions about which events count, how to handle reorgs, how to classify a payment as "agent-driven" vs something else. Those are methodology questions disguised as engineering questions, and they deserve real care.

The endpoint registry is even simpler: a YAML file curated by hand initially, a Go service that probes each endpoint weekly, records what it sees, and stores the history. The "by hand initially" part is correct — know every endpoint personally for the first few months. Automation comes after the shape of the data is understood.

## Layer 2: The warehouse

This is one Postgres database, deliberately simple. Three core tables:

**`payments`** — one row per observed payment. Columns: id, observed_at, source (which collector), chain, protocol, payer_address, payee_address, payee_service_id (nullable, filled in when the payee matches a known service), amount, asset, asset_usd_at_time, raw_tx_ref.

**`services`** — one row per known 402-speaking service. Columns: id, url, first_seen, last_probed, status, declared_methods (jsonb), price_observations (jsonb), category, notes.

**`probe_results`** — historical record of every probe of every endpoint in `services`. Append-only. This gives the time-series of the supply side.

The methodology magic happens in **views**, not in the raw tables. Write SQL views like `weekly_payment_volume`, `weekly_new_services`, `weekly_active_services`, `weekly_payer_concentration`. Each view encodes a methodology decision. When changing a methodology, create a new view (`weekly_payment_volume_v2`) rather than mutating the old one. This gives full reproducibility: anyone asking "what did your numbers say in March" can run the views as they existed in March.

This is the most important technical decision in the whole system. Methodology versioning via views is what makes the index defensible. Get it right early.

## Layer 3: The publisher

Boring on purpose. A Go program that:

- Reads the views for the current week
- Generates a JSON snapshot, a CSV, and a Markdown summary
- Commits all three to a public GitHub repo with a tagged release per week
- Generates the social-media post (text + chart images) into a draft folder for manual review and posting

**Do not automate the actual posting to X.** Read every weekly update before it goes out, for the first six months minimum. The credibility of the index depends on judgment being in the loop. Once the audience trusts the numbers, automation can come. Not before.

For charts, use Go's standard plotting libraries or shell out to a small Python script. Charts are part of the brand — pick a visual style early (color palette, font, layout) and never deviate. Consistency of visual identity is part of the moat.

The public GitHub repo is doing real work here: it's version control, methodology disclosure, reproducibility argument, and distribution channel for the data feed simultaneously. Open the repo from day one. The data being public is what makes citation easy and freeloaders irrelevant — the value isn't the data, it's the consistency and the interpretation.

## What v1 actually looks like, concretely

**Week one:**

- Postgres + Go project skeleton, one repo, deployed to a single VPS
- Base x402 collector running, backfilled to the protocol's launch
- Hand-curated YAML of ~20 known 402 endpoints, probe service running daily
- Public GitHub repo for the data outputs (separate from the code, or a subfolder)

**Week two:**

- First weekly publication. Three numbers: total Base x402 payment volume, count of active services, count of new services this week. One chart per number. Two paragraphs of interpretation.
- Solana x402 collector started

**Week three or four:**

- Solana numbers added to the publication
- First long-form post: "How I'm measuring the agent economy — methodology v1"

**Month two:**

- Expand endpoint registry to 50+ services
- Add one composition metric (e.g., volume by service category)
- First piece of decision-relevant analysis ("most x402 volume is concentrated in N actors — what that means")

**Month three:**

- All-402 reframe: start ingesting L402/Lightning or another protocol. Now it's an "agent economy" index, not an x402 index.
- Methodology v2 published. First time defending a methodology change publicly.

## The technical decisions worth making now

A few choices to lock in early because changing them later is expensive:

**On naming.** Pick a name that's broader than x402. "x402intel" anchored to one protocol. Something like *agentflow*, *agentdex*, *protocol402*, *the agent economy index* — pick something defensible at month 24 when the index covers six protocols. Don't agonize for weeks, but don't pick something narrow.

**On open source.** The collectors and the methodology views should be public. Editorial commentary, client list, unreleased analyses are private. This is the same model Glassnode and Nansen use — open enough to be trusted, closed enough to have a business.

**On hosting.** Single VPS for everything for now. No Kubernetes, no managed services beyond Postgres. The system needs to be operable in 30 minutes a week. Optimize for that, not for scale that doesn't exist.

**On the indexing approach.** Resist the urge to build a beautiful event-sourcing system or use a fancy framework. Postgres + cursors + simple Go services is the right answer for years. The boring choice is the correct choice here because the scarce resource is editorial attention, not engineering time.

## What you should explicitly *not* build in v1

- A web frontend. The GitHub repo + X posts are enough. A site comes at month four or five when there's something worth displaying.
- An API. Same logic. The CSV/JSON in the repo IS the API for now.
- A database other than Postgres. No Clickhouse, no DuckDB, no TimescaleDB extensions, no Kafka. Not yet.
- Authentication, user accounts, billing. There's no product to sell yet.
- A separate "agent identification" system. For v1, every payment from a 402 transaction counts as "agent-mediated." The definitional purity questions come later.

## The one big thing to internalize

The interesting technical work is not in the collectors. It's in the **methodology decisions**, which get made for the first time when each SQL view gets written. "Should this address count as agent-mediated?" "Should we include test transactions if they're under $0.01?" "How do we handle a service that briefly appeared and disappeared?" Each of those questions is where domain judgment shows up, and the quality of that judgment is what people will pay for eventually.

Treat the engineering as plumbing. Treat the methodology as the product.
