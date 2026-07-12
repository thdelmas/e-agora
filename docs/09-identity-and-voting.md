# 09 — Identity, Sybil-Resistance & Voting

A decision reference for a question that grows with the project: **how do we know a
vote came from a distinct human, without learning who that human is?** Today
e-agora is a casual, for-fun politician-rating board ([05](05-ranking.md)); the
stated ambition is to become a *real* voting system. Those are not the same
problem, and the gap between them is the subject of this document.

The one sentence to take away: **proof-of-humanity solves at most a third of what a
real election needs.** A "prove you're a unique human" service gives you
*uniqueness* and (sometimes) *ballot secrecy*. A binding election also needs
*eligibility*, *coercion-resistance*, and *end-to-end verifiability* — and the
research consensus (§4) is that the last mile, binding **internet** voting, is not
securable with any technology known as of 2026. This doc lays out the options on
both halves with their trade-offs, then gives an e-agora-specific recommendation.

> **Sourcing & confidence.** This doc was assembled from a fan-out, adversarially
> fact-checked research pass. Claims that survived 3-vote verification carry an
> inline source and are marked **[verified]**. The proof-of-humanity landscape
> beyond World ID and Human Passport (BrightID, Idena, Kleros PoH, zkPass, Privado
> ID, Anima, Civic) is drawn from **general knowledge, not independently verified
> here** — marked **[unverified]** — and should be re-checked before you rely on
> it. This space moves fast; treat every regulatory and version detail as
> perishable (see §5).

---

## 1. The five properties a real election needs

Conflating these is the classic trap. A fun rating board needs only the first two
rows; a binding election needs all five, and the hard ones are rows 4–5.

| Property | Question it answers | Covered by proof-of-humanity? |
|----------|---------------------|-------------------------------|
| **Eligibility** | May *this* voter vote in *this* election (citizen of a district, member of an org)? | No — this is the part fundamentally at odds with "anonymous to the verifier" |
| **Uniqueness** | One person, one vote — no ballot-stuffing / sybils? | **Yes** — this is what PoH services actually deliver |
| **Ballot secrecy** | Does anyone learn *how* you voted? | Partially — some PoH designs are unlinkable, but secrecy of the *tally* is an election-system property |
| **Coercion-resistance / receipt-freeness** | Can you *prove* how you voted (and thus be bought or intimidated)? | No |
| **End-to-end verifiability (E2E-V)** | Can a voter confirm their vote was counted, and anyone confirm the tally, without breaking secrecy? | No |

A sixth, **auditability** (a meaningful way to recover when something goes wrong),
rides on E2E-V and turns out to be the crux of the internet-voting problem (§4).

---

## 2. The two-axis privacy model

"Anonymous" splits into two independent properties. Almost every disappointment in
this space comes from assuming a service gives you the second when it only gives
the first.

- **Anonymous to the relying app** — the app you log into learns "unique human"
  but not *who*, and can't correlate you across apps or actions. The standard
  mechanism is a **nullifier**: an opaque token `f(identity, context)` that is
  *stable* for a given (person, context) — so a second vote on the same matchup
  reproduces the same token and is rejected — but *unlinkable* across different
  contexts and back to the identity. This is exactly the shape a vote-dedup layer
  wants.
- **Anonymous to the verifier itself** — the service that certifies you're human
  never learns your identity (no biometrics, no government ID, no PII retained).

The verified finding worth internalizing: **the mature PoH services give you the
first, not the second.** World ID is anonymous-to-the-relying-app via unlinkable
nullifiers but collects biometrics at the Orb; Human Passport is
anonymous-to-the-relying-app via zero-knowledge proofs but a regulated provider
still sees your PII at verification time. Neither is anonymous-to-the-verifier.

---

## 3. Part A — Anonymous proof-of-humanity services

### 3.1 World ID (Worldcoin / Tools for Humanity) **[verified]**

**Mechanism.** A hardware device (the "Orb") captures iris and face biometrics. By
default the raw images are processed in local memory and destroyed; what is
retained is the derived **iris code** — a biometric *template* — used for the
global uniqueness check.<sup>[1][2]</sup> Apps then receive a per-context
**nullifier** built on the Semaphore construction: `nullifier = f(private_key,
app_id [+ action])`. Distinct apps get unlinkable nullifiers; the same
person+context always reproduces the same one.<sup>[1][3]</sup>

**Privacy model.** *Anonymous to the relying app* — the nullifier "cannot be linked
to other nullifiers from the user, nor their identity commitment, therefore
preventing cross-application tracking."<sup>[1]</sup> *Not anonymous to the
verifier* — a biometric template is collected and retained. (V4 strengthened
cross-app unlinkability with `rp_id` scoping and one-time-use nullifiers via a
TACEO OPRF / "Oblivious Nullifier Pool"; in V3, two apps colluding with an
identical context could in principle correlate a user.<sup>[3]</sup>)

| | |
|---|---|
| **Pros** | Strongest *uniqueness* guarantee of any option here (hard biometric); nullifier model is a near-drop-in for anonymous one-person-one-vote; cloud verification + IDKit SDK make web integration straightforward; free. |
| **Cons** | **Requires biometrics** — the single biggest adoption and ethics objection; the iris-code database is centralized; **legally contested** (see below); availability differs by country; ties your app to a controversial, well-funded crypto venture. |

**Regulatory status (perishable).** Regulators have ordered deletions or found
violations in **Germany** (BayLDA), **Kenya** (High Court; suspended), and
**Colombia**, generally over centralized iris-code storage, consent/legal-basis
gaps, missing DPIAs, and cross-border transfers; other jurisdictions have paused or
investigated it.<sup>[1][4]</sup> Under EU/GDPR, biometric data is a *special
category* with a high bar — directly relevant for a France-based project. **Verify
current status per target jurisdiction before integrating.**

> **Myth to avoid (refuted 0–3).** It is *not* true that World ID's zero-knowledge
> proofs prevent World itself from learning your identity commitment or iris code.
> The ZK layer hides you from *relying apps*, not from the verifier. Don't market
> or design around "World can't see who I am."

### 3.2 Human Passport — Human.Tech (formerly Gitcoin Passport; Holonym) **[verified]**

**Mechanism.** Attribute *aggregation* rather than one binding biometric. Users
collect **Stamps** — verifiable credentials for attributes like on-chain activity,
government ID via **zk-KYC**, optional biometrics (3D liveness), social accounts,
and web-of-trust — and a scorer sums their weights into a composite **Unique
Humanity Score**.<sup>[5][6]</sup>

**Privacy model.** *Anonymous to the relying app* — "apps learn that a user passed
KYC, not their name or ID number," and the credential is stored client-side so
"personal data never leaves the user's device."<sup>[5][6]</sup> *Not anonymous to
the verifier* — the underlying government-ID check is run by a regulated provider
that does see PII at verification time.

| | |
|---|---|
| **Pros** | No single hard biometric required; tunable threshold lets *you* pick how strict; flexible mix of signals; PII stays client-side; less ethically fraught than iris scanning. |
| **Cons** | **Weaker, gameable uniqueness** than a hard biometric — a determined sybil can farm multiple stamp sets; score thresholds need tuning; ZK implementation rests on **vendor self-description, no independent crypto audit**; still web3-flavored (wallets, on-chain stamps). |

### 3.3 The rest of the landscape **[unverified — general knowledge]**

Not verified in this research pass; included for completeness, re-check before
relying. Evaluate each on the three axes from §2 + §3: *(a) biometrics/PII at the
verifier? (b) unlinkable cross-app nullifiers? (c) uniqueness strength vs
decentralization vs regulatory exposure.*

- **BrightID** — uniqueness from a *social graph* (verification connections/meets).
  No government ID, no biometrics → genuinely privacy-preserving, pseudonymous. The
  trade-off is the softest sybil guarantee here and awkward onboarding.
- **Idena** — proof-of-person via synchronized CAPTCHA-like "flip" ceremonies all
  participants solve at once. No PII; you're just a keypair. Strong liveness story,
  niche adoption, recurring-validation friction.
- **Proof of Humanity (Kleros registry)** — despite the name, profiles are
  **public** (video + name), adjudicated by the Kleros court. Good sybil
  resistance, **the opposite of anonymous** — listed here precisely so it isn't
  mistaken for an anonymity solution.
- **zkPass** — prove attributes from existing web/credentials in zero-knowledge
  (MPC-TLS style). Privacy-strong for *attribute* proofs; uniqueness depends on
  what you prove against.
- **Privado ID (ex-Polygon ID)** / **Anima** — self-sovereign identity with
  selective-disclosure ZK credentials. Toolkits/issuer frameworks more than a
  ready-made human-uniqueness oracle; you supply the issuer.
- **Civic** — identity verification leaning KYC/compliance; more "verified
  identity" than "anonymous personhood."

### 3.4 Comparison

| Service | Uniqueness | Biometric? | PII at verifier? | Cross-app unlinkable? | Maturity | Confidence |
|---------|-----------|-----------|------------------|----------------------|----------|-----------|
| World ID | Very strong | **Yes (iris)** | Yes (iris code) | Yes (nullifiers) | High, contested | **[verified]** |
| Human Passport | Medium (tunable) | Optional stamp | Yes (zk-KYC) | App-scoped | Medium | **[verified]** |
| BrightID | Soft | No | No | Pseudonymous | Low–medium | [unverified] |
| Idena | Medium | No | No | Pseudonymous | Low | [unverified] |
| Kleros PoH | Strong | No | **Public profile** | **No (doxxed)** | Medium | [unverified] |
| zkPass | Depends | No | Minimized (ZK) | Depends | Low–medium | [unverified] |
| Privado ID / Anima | Depends (issuer) | No | Depends (issuer) | Selective-disclosure | Low–medium | [unverified] |
| Civic | Medium | Optional | **Yes (KYC)** | No | Medium | [unverified] |

---

## 4. Part B — Privacy-preserving election systems

These are the systems that actually deliver rows 3–5 of §1. They share a small set
of cryptographic building blocks and one architectural pattern.

### 4.1 Building blocks

- **Homomorphic tallying** — encode votes as additively-homomorphic (exponential)
  ElGamal ciphertexts; combine them *while encrypted* so only the **aggregate** is
  ever decrypted. Individual ballots stay sealed.
- **Mixnets / verifiable shuffles** — an alternative to homomorphic tally for
  expressive ballots (rankings): shuffle encrypted ballots through several mixers,
  each proving it permuted without altering, then decrypt the shuffled set.
- **Threshold / distributed decryption** — split the decryption key across *n*
  trustees so *k* must cooperate; no single party can open ballots.
- **Blind-signature / anonymous credentials** — let an authority certify "eligible
  voter" without learning, or being able to link, the ballot that credential later
  signs.
- **ZK validity proofs** (e.g. disjunctive Chaum-Pedersen) — prove a ciphertext
  encodes a *legal* vote (0 or 1, sums to 1) without revealing it.

### 4.2 The separation-of-duties pattern

The key idea that reconciles **eligibility** with **anonymity**: split the
**eligibility authority** (a registrar that certifies who may vote and issues an
anonymous/unlinkable credential) from the **ballot box** (which accepts votes
authenticated by that credential but learns nothing about identity). Done right, a
*fully compromised ballot box still cannot stuff votes*, because it never holds the
private credentials. This is the pattern e-agora should design its data model
around (§6) — even before adopting any specific system.

### 4.3 The systems

**Helios** — the web-based E2E-V workhorse; used for real organizational elections
(e.g. the IACR board). Cast-or-audit (Benaloh challenge) + homomorphic tally. Open,
mature, *not* coercion-resistant (a voter can prove their vote), so suited to
low-coercion settings like societies and boards. *[Background; corroborated as the
lineage Belenios/ElectionGuard descend from.]*

**Belenios (INRIA)** **[verified]** — extends Helios with an explicit **registrar**
that issues each voter a signing credential and gives the server only the public
verification keys, so "even if the election server is compromised, it is impossible
to add valid ballots."<sup>[7][8]</sup> Ballot secrecy via additive-homomorphic
ElGamal — "the ballot of an individual voter is never decrypted."<sup>[7]</sup>
End-to-end verifiable (individual + universal). Machine-checked (EasyCrypt) security
proofs; actively maintained; used in thousands of elections.

- *Caveats:* the anti-stuffing guarantee assumes the registrar and server **don't
  collude** (security holds "as soon as either the voting registrar or the voting
  server is honest"<sup>[8]</sup>); final decryption needs cooperating threshold
  trustees, so secrecy rests on trustees not all colluding; the mixnet mode
  *does* decrypt shuffled ballots; cast-as-intended verifiability arrived later
  (Belenios-CaI, 2023).
- > **Myth to avoid (refuted 1–2).** Don't state that Belenios verifiability "holds
  > even when the voting server is corrupted" as its headline advance over Helios.
  > Report the properties above as scoped, not the overreaching version.

**Microsoft ElectionGuard** **[verified]** — *not a voting system* but an open
**E2E-V SDK** that, "when used with traditional election systems," produces
verifiable tallies; in its preferred deployment it "runs alongside" existing
counting rather than replacing it.<sup>[9]</sup> Crypto: exponential ElGamal in
`Z*_p`, additive-homomorphic tally, disjunctive Chaum-Pedersen validity proofs,
*k-of-n* threshold "guardian" decryption. Integrity holds even against a coalition
of *all* guardians plus the administrator; **secrecy** still relies on the
guardians.<sup>[9]</sup>

- *Critical gap:* ElectionGuard **explicitly does not handle eligibility /
  authentication** — it can verify the *number* of ballots but not their origin;
  "when paired with an external list of people who voted, this enables full
  verifiability."<sup>[10]</sup> You must bolt on an identity/eligibility layer
  (e.g. a Part-A service or a poll-book).
- *Authors' own caveat:* "**not recommended for public elections**"; primary use is
  **in-person poll-site** voting; online use "would resemble Helios" — and they cite
  the National Academies report against internet voting.<sup>[11]</sup>

**Civitas (Cornell)** **[verified]** — the first *implemented* system that is
simultaneously **coercion-resistant**, universally + voter verifiable, and suitable
for remote voting; refines the JCJ scheme.<sup>[12]</sup> Coercion-resistance comes
from **voter-substituted fake credentials**: a voter can locally fabricate fake
credentials indistinguishable to a coercer from real ones, comply with the coercer
using the fake, and have their *real* vote still count — which also defeats
vote-selling.<sup>[13]</sup> Trust is distributed (registration tellers + threshold
tabulation tellers + separate bulletin board).<sup>[14]</sup>

- *Why it's not a drop-in:* coercion-resistance rests on **anonymous channels +
  untappable channels** (perfect-secrecy, one-time-pad-grade) and the assumption an
  adversary can't control a voter for the whole election — assumptions the authors
  themselves call an open problem to relax.<sup>[15]</sup> Later work (Cortier et
  al., 2022) shows JCJ/Civitas leak information during ballot-cleansing, qualifying
  the coercion-resistance strength.

### 4.4 Comparison

| System | Eligibility built in? | Secrecy | E2E-V | Coercion-resistant? | Best fit |
|--------|----------------------|---------|-------|---------------------|----------|
| Helios | Light (login) | Homomorphic | Yes | No | Low-coercion org/board votes |
| Belenios | **Yes (registrar credential)** | Homomorphic (or mixnet) | Yes | No | Community/org elections needing anti-stuffing |
| ElectionGuard | **No (external)** | Threshold guardians | Yes | No (receipts can enable it) | Verifiability *add-on* to poll-site systems |
| Civitas | Yes (registration tellers) | Threshold | Yes | **Yes (JCJ fake creds)** | Research-grade remote coercion-resistance |

---

## 5. The hard truth: binding internet voting

The decisive finding of the research, and the boundary on e-agora's ambition:

- **Decades-long, currently-reaffirmed consensus.** A January 2026 letter from 21
  computer scientists (incl. Rivest, Schneier, Halderman, Appel, Dill) states it
  has "been the scientific consensus for decades that internet voting is not
  securable by any known technology."<sup>[16]</sup> The U.S. National Academies
  (*Securing the Vote*, 2018): "no known technology … can guarantee the secrecy,
  security, and verifiability of a marked ballot transmitted over the
  Internet."<sup>[17]</sup> AAAS: all internet voting is "currently inherently
  insecure," with "no technical evidence" it can be made safe "in the foreseeable
  future."<sup>[18]</sup>
- **Client-side malware.** Malware on the voter's own device can silently swap the
  vote *after* they review it, and "there is no known technical solution that can
  fully protect an unsupervised endpoint" — a point even internet-voting proponents
  concede.<sup>[16][17]</sup> This is the single most decisive obstacle.
- **Verifiability vs. secrecy is a genuine tension.** Receipt-freeness is required
  to stop automated mass vote-buying, yet the best cast-as-intended schemes only
  let a voter verify ballots that will be *discarded* while casting ones that were
  never checked (Benaloh cast-or-audit). A 2024 impossibility result proves
  receipt-freeness + cast-as-intended is impossible for non-interactive voting
  without a trusted authority.<sup>[16][19]</sup>
- **No paper, no audit.** Absent a voter-verified (typically paper) record, there
  is no way to run a valid corrective audit; cryptographic *detection* of fraud is
  not the same as a working dispute-resolution mechanism.<sup>[16][18]</sup>
- **Documented failures** reinforce this: the 2019 Swiss e-voting vulnerability,
  the OmniBallot/Democracy Live analysis (Specter & Halderman), and others.

**Scope it correctly.** The consensus is about *currently known technology* / the
*foreseeable future* and about *legally binding public elections* — **not** a claim
of permanent impossibility, and **not** a verdict against organizational/
community-scale verifiable elections, which the same cryptography handles well.

---

## 6. Recommendation & phased roadmap for e-agora

The honest framing: **org/community-scale verifiable elections are achievable and a
great roadmap; binding government internet voting is not e-agora's to solve** —
that's a legal/governance/public-trust problem the research community advises
against attempting online. Aim e-agora at the achievable target and be explicit
about the ceiling.

**Phase 0 — now (casual).** Don't gate on heavy proof-of-humanity yet. Use cheap,
privacy-preserving sybil resistance: rate-limiting, a privacy-respecting bot check
(Turnstile-style), anonymous per-session dedup. This stops the realistic threat
(scripts, casual double-voting) at near-zero friction, consistent with the existing
no-profiling stance in [05](05-ranking.md).

**The forward-compatible move (do this now, costs little).** Store each vote against
an opaque **`voter_token` shaped like a nullifier** — `f(identity, context)` — *not*
a user id. Keep the eligibility/issuer side abstract: "an opaque proof of (eligible
+ unique)." Then the issuer behind that token can evolve `nothing → World ID /
Human Passport → a real registrar` **without a schema migration**, and the data
model stays anonymous by construction (also keeping you out of GDPR PII scope). This
mirrors the separation-of-duties pattern (§4.2).

**Phase 1 — community/org-scale verifiable elections.** When results need to be
credible, adopt a **Belenios-style** design: a separate registrar issues anonymous
signing credentials (anti-stuffing), additive-homomorphic ElGamal keeps ballots
sealed, threshold trustees decrypt only the tally, and the bulletin board gives
E2E-V. For *uniqueness* feeding the registrar, layer a Part-A service — **World ID**
if you need the hardest one-person-one-vote and accept the biometric/regulatory
baggage, **Human Passport** if you want lower friction and tunable strictness.
Consider running World ID-verified votes as a separate "verified human" tally rather
than hard-gating everyone, so casual users aren't locked out.

**The ceiling — binding public elections.** Treat as out of scope for the software.
If e-agora's numbers are ever cited as authoritative, the value is in *verifiable
community polling*, not in claiming a property (binding, coercion-resistant,
auditable internet voting) that the field says can't currently be delivered.

**Open questions worth a follow-up** (carried from the research): concrete SDK
effort to wire World ID / Human Passport into the Vue + Go stack; who would serve as
independent trustees/registrar for a Belenios deployment and at what *k-of-n*; and
whether the post-JCJ coercion-resistance schemes (e.g. CHide) are implemented and
maintained as of 2026.

---

## Sources

Primary/verified sources behind the **[verified]** claims above:

1. World ID privacy deep-dive — <https://world.org/blog/developers/privacy-deep-dive>
2. Trail of Bits security audit (commissioned by Tools for Humanity, 2023)
3. World ID protocol / Semaphore — github.com/worldcoin/world-id-protocol, semaphore-rs
4. German regulator (BayLDA) deletion order coverage; Kenya & Colombia regulatory actions
5. Human Passport / Human.Tech — <https://human.tech/blog/human-passport-proof-of-personhood-and-sybil-resistance-for-web3>
6. Passport docs — <https://docs.passport.xyz/>
7. Belenios — how it works — <https://www.belenios.org/howitworks.html>
8. Cortier et al., machine-checked Belenios proofs — <https://link.springer.com/chapter/10.1007/978-3-030-19052-1_14> (IACR ePrint 2021/1201)
9. Benaloh et al., "ElectionGuard," USENIX Security 2024 — <https://www.usenix.org/system/files/usenixsecurity24-benaloh.pdf> (IACR ePrint 2024/955)
10. ElectionGuard eligibility scope — same paper, §on eligibility verifiability
11. ElectionGuard "not recommended for public elections" — same paper, citing NAS
12. Clarkson, Chong, Myers, "Civitas," IEEE S&P 2008 — <https://www.cs.cornell.edu/projects/civitas/papers/clarkson_civitas_tr.pdf>
13. Civitas fake-credential coercion-resistance — same paper (JCJ mechanism)
14. Civitas distributed trust (registration + tabulation tellers) — same paper
15. Civitas channel assumptions (anonymous + untappable) — same paper, §assumptions
16. CITP letter, "Internet voting is insecure," 2026-01-16 — <https://blog.citp.princeton.edu/2026/01/16/internet-voting-is-insecure-and-should-not-be-used-in-public-elections/>
17. National Academies, *Securing the Vote*, 2018 — <https://nap.nationalacademies.org/catalog/25120>
18. AAAS EPI Center, internet/online voting — <https://www.aaas.org/epi-center/internet-online-voting>
19. INRIA/IEEE 2024 impossibility result, cast-as-intended vs receipt-freeness — hal-04371905

*Last researched 2026-06; regulatory and protocol-version details (esp. World ID)
are time-sensitive — re-verify per jurisdiction before integrating.*
