# 06 — Data Ingestion (Wikidata + Wikipedia)

Requirement R2: **every subject must have a Wikipedia page.** R1.1: seed with
**every UN-country leader** (head of state + head of government). R8: visitors
may add **any human** with a Wikipedia page. R9: subjects must be renderable in
multiple languages. This doc defines how the pool is sourced, identified, and
localized.

Two upstreams, both contacted **only** at seed/add time (never on the hot path):
- **Wikidata** — the *identity & enumeration* layer (QIDs, "is a human", UN
  leaders, the set of language editions a subject has).
- **Wikipedia REST** — the *content* layer (per-language name, description,
  image, page URL).

## Principle

- A subject is anchored by its **Wikidata QID** (e.g. `Q567`) — one person, one
  row, across all languages (R9). Dedup is by QID.
- A subject exists only if it resolves to a real Wikipedia page (R2). Anything
  unresolvable is **skipped and logged**, never inserted with a fabricated URL.
- After seeding, the app needs only PostgreSQL; upstreams are touched again only
  for user-adds and lazy translation fills.

## Endpoints used

| Purpose | Endpoint |
|---------|----------|
| Enumerate UN leaders | Wikidata SPARQL: `https://query.wikidata.org/sparql?format=json&query=…` |
| Entity facts (labels, sitelinks, `P31`) | `https://www.wikidata.org/wiki/Special:EntityData/{QID}.json` |
| Per-language summary (name/description/image/url) | `https://{lang}.wikipedia.org/api/rest_v1/page/summary/{title}` |
| Add-autocomplete (search) | `https://{lang}.wikipedia.org/w/rest.php/v1/search/page?q=…` |

**Etiquette (required)**: send a descriptive **`User-Agent`**
(`e-agora/0.1 (https://github.com/<owner>/e-agora; contact@example.com)`); issue
requests **sequentially** with a small delay (~100–200 ms); retry `5xx` with
backoff; follow title redirects; treat `404` as a skip. Wikimedia may block
generic/empty agents.

## Step 1 — Enumerate UN-country leaders (Wikidata)

A SPARQL query yields the current head of state (`P35`) and head of government
(`P6`) of every UN member state, as person QIDs:

```sparql
SELECT DISTINCT ?country ?countryLabel ?person ?personLabel WHERE {
  ?country wdt:P463 wd:Q1065 .            # member of: United Nations
  { ?country p:P35 ?st } UNION { ?country p:P6 ?st }   # head of state / government
  ?st ps:P35|ps:P6 ?person .
  FILTER NOT EXISTS { ?st pq:P582 ?end }  # exclude statements with an end date (former)
  SERVICE wikibase:label { bd:serviceParam wikibase:language "en". }
}
```

- **193 UN members** come from `P463 = Q1065`. The **2 observer states** (Holy
  See `Q237`, State of Palestine `Q219060`) aren't `P463` members — add their
  leaders explicitly by country QID (R1.1).
- **Current only**: prefer truthy/preferred-rank statements and exclude those
  with an end-time qualifier (`P582`) to drop former office-holders.
- **Dedupe persons**: a person who is both HoS and HoG (or shared across roles)
  appears once — dedup by person QID.

**Reproducibility & offline**: the query result is cached to a committed
snapshot `backend/data/un_leaders.json`
(`[{ "qid": "Q…", "country": "…", "name": "…" }]`; `name` is a label hint used
for logs and the offline fallback), embedded into the binary and used as the
default seed input. Re-running the query (a maintenance task) refreshes the
snapshot when leaders change.

## Step 2 — Resolve identity, languages, and English content

For each person QID, fetch `Special:EntityData/{QID}.json` and read:

| Wikidata field | → e-agora | Notes |
|----------------|-----------|-------|
| `claims.P31` contains `Q5` | (validation) | **must be a human** (R8); else skip & log |
| `labels.en.value` | `subjects.canonical_name` | default/fallback display name |
| `sitelinks` (`*wiki` keys) | `subjects.available_langs` | `enwiki`→`en`, `frwiki`→`fr`, … (drives R9) |
| `claims.P27`/country context | `subjects.country` | best-effort (seed sets it from the SPARQL country) |

Then fetch the **English** summary
(`https://en.wikipedia.org/api/rest_v1/page/summary/{enwiki title}`) and store
the `(subject, 'en')` translation:

| Summary field | → `subject_translations` |
|---------------|--------------------------|
| `titles.normalized` / `title` | `name` |
| `description` (fallback: `extract`) | `description` |
| `thumbnail.source` | `image_url` (nullable) |
| `content_urls.desktop.page` | `wikipedia_url` (**mandatory**, R2) |

English is always cached at seed so the R9 fallback ("English for both") always
has content. Other languages are filled **lazily** (Step 4).

## Step 3 — Seeding algorithm (startup)

Controlled by `EAGORA_SEED`: `auto` (seed only if `subjects` is empty — default),
`off` (never contact upstreams), `force` (re-ingest/upsert, refreshing metadata
but **preserving ratings & vote history**).

```
load un_leaders.json   (+ optional data/seed_extra.json hand-picked humans)
for each { qid, country }:
    entity = GET EntityData(qid)
    if Q5 not in entity.P31:               log skip "not a human"; continue
    langs = languages(entity.sitelinks)
    if "en" not in langs:                  log skip "no English page"; continue   # R9 fallback needs it
    upsert subjects on wikidata_id:
        canonical_name = labels.en, country, source='seed', available_langs=langs
        # never touch rating/wins/losses/comparisons on refresh
    en = GET summary(en, enwiki-title)
    upsert subject_translations (subject_id, 'en') = {name, description, image_url, wikipedia_url}
    sleep ~150ms
log: N enumerated, M upserted, K skipped (with reasons)
```

Each upsert is its own small transaction → a mid-run failure leaves a partial
-but-valid pool and the next run resumes idempotently.

## Step 4 — Lazy per-language translations (R9)

At request time, the matchup/leaderboard handler needs subject content in a
`displayLang`:
1. it already knows `available_langs` (cheap column), so it can apply the R9
   rule without any remote call;
2. if `(subject, displayLang)` is **not** yet in `subject_translations`, fetch
   `https://{displayLang}.wikipedia.org/api/rest_v1/page/summary/{title-in-that-lang}`
   (the title comes from the sitelink), insert the row, then serve;
3. subsequent requests for that language are served from cache.

Only languages that are actually requested are ever fetched/stored, keeping the
translations table small.

## User-added subjects (R8)

`POST /api/subjects` accepts a Wikipedia URL, a `wikidataId`, or a search-picked
title. It is **gated by a valid access token and limited to one add per token**
(R8.1; enforced via the `subject_add_log` `jti` ledger — see
[04](04-api.md)/[03](03-data-model.md)). Pipeline:

```
require valid access token (else 403); jti not already used (else 429)
resolve input → Wikidata QID
    (URL → page → wikibase_item; or use the title's summary `wikibase_item`)
GET EntityData(qid)
assert Q5 ∈ P31                      else 422 not_a_person          (R8)
assert sitelinks has a *wiki page    else 422 not_a_wikipedia_page  (R2)
dedupe by wikidata_id                else 409 already_exists
langs = languages(sitelinks)
insert subjects(source='user', canonical_name=labels.en or best label, available_langs=langs, rating=1500)
fetch + insert English translation (or best-available if no en — see Q6)
201 { subject }
```

The autocomplete (`GET /api/subjects/search`) proxies Wikipedia search so the
visitor can disambiguate before adding; QID resolution and the human-check
happen server-side at add time (clients can't be trusted, R-ADD1).

## Offline / failure fallback

If upstreams are unreachable at seed time (CI, air-gapped dev), seed a
**minimal valid** pool from `un_leaders.json` alone:
- `subjects`: `wikidata_id=qid`, `canonical_name` = a name hint from the
  snapshot, `country`, `available_langs={'en'}` (assumed);
- `subject_translations(qid,'en')`: `name` = hint,
  `wikipedia_url = https://en.wikipedia.org/wiki/Special:EntityData/{qid}`’s
  linked article *if known*, else a `Special:GoToLinkedPage`-style URL that still
  resolves to the real page; `description`/`image_url` = null.
- Log a clear warning that metadata is degraded; a later `EAGORA_SEED=force`
  re-ingest fills it in.

This keeps the app runnable end-to-end while still honoring "every subject has a
Wikipedia page."

## Validation & quality gates

- **Human only**: `Q5 ∈ P31` for seed leaders and user-adds (R8); skip/reject
  otherwise (a country/party/event QID must never enter the pool).
- **Has a Wikipedia page** (R2); English page required for the R9 fallback (rare
  exceptions handled per Overview Q6).
- **Dedup by QID** — the API normalizes titles, and QID collapses
  cross-language/alias duplicates.
- Log every skip/reject with a reason so the snapshot/curated extras can be
  corrected.

## Re-ingestion & maintenance

- **Add leaders / refresh after elections**: re-run the SPARQL query → update
  `un_leaders.json` → `EAGORA_SEED=force` (existing subjects keep ratings; new
  ones start at 1500).
- **Add hand-picked humans**: append to `data/seed_extra.json`, run `force`.
- **Hide a subject** without losing history: `active=false` (admin/CLI; no public
  endpoint in v1).
- **Refresh stale translations**: evict by `fetched_at` and let Step 4 refill, or
  `force`.

## Out of scope for v1

- Auto-tracking leadership changes (a scheduled re-query/cron).
- Per-visitor UI-chrome translation beyond a small bundled set ([01](01-functional-spec.md) §i18n).
- Local image caching / explicit attribution UI beyond the page link.
- Wikidata rank/qualifier edge cases beyond "exclude statements with an end date".
