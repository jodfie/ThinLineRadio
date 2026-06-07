# Auto-learn tone sets

Learn paging tone patterns from live traffic, then email system admins for manual review. **Never auto-adds** tone sets to a talkgroup.

## Goals

- Detect **A+B pairs** or **long tones** on calls where auto-learn is enabled.
- After the **same signature** appears on **N distinct voiced calls** (default 3), email system admins with **MP3 audio** and **transcripts**.
- Use **OpenAI** once with **all N transcripts** to suggest a department/station label.
- **Skip entirely** if an equivalent tone set already exists on that talkgroup.

## Enablement

| Level | Field | Default |
|-------|--------|---------|
| System | `autoLearnToneSets` | `false` |
| Talkgroup | `autoLearnToneSets` | `false` |

Active when **both** are on and system/talkgroup `alertsEnabled` are true.

## Learnable patterns

### A+B pair

Both tones must appear on the same call as a sequential pair. Durations must fall within configured windows (system-level):

| Setting | Default |
|---------|---------|
| `autoLearnAToneMinDuration` | 0.5 s |
| `autoLearnAToneMaxDuration` | 0.9 s |
| `autoLearnBToneMinDuration` | 1.5 s |
| `autoLearnBToneMaxDuration` | 2.5 s |

### Long tone

Single sustained tone:

| Setting | Default |
|---------|---------|
| `autoLearnLongToneMinDuration` | 6.0 s |
| `autoLearnLongToneMaxDuration` | 0 (unlimited) |

### Stacked tones

One call may produce **multiple** candidates (one per valid pair or long tone in the sequence).

### Not learnable

- A without B (or B without A) for pair patterns.
- Tones outside duration windows.
- Signatures matching an existing talkgroup tone set.
- Calls without voice in the transcript.
- Cross-channel paging (out of scope v1).

## Other system options

| Setting | Default | Purpose |
|---------|---------|---------|
| `autoLearnCallsRequired` | 3 | Distinct voiced calls before review email |
| `autoLearnFrequencyToleranceHz` | 10 | Signature matching and duplicate detection |

## Candidate storage

Table `toneSetLearnCandidates`:

- `systemId`, `talkgroupId`, `signatureHash`, `patternType` (`ab_pair` | `long`)
- `toneSetDraft` (JSON), `callIds` (JSON array), `callCount`
- `firstSeenAt`, `lastSeenAt`, `reviewEmailedAt`

No `rejected` status. No automatic writes to `talkgroups.toneSets`.

## Pipeline

1. Transcription completes on an auto-learn talkgroup.
2. Require voice in transcript.
3. Extract valid A+B pairs / long tones (FFT discovery + duration windows).
4. For each signature: if duplicate of existing tone set → skip.
5. Upsert candidate with distinct `callId`.
6. When `callCount >= autoLearnCallsRequired` and every stored call has voice:
   - OpenAI: all transcripts + tone metadata → suggested label.
   - Email system admins: body with transcripts + **3 MP3 attachments**.
   - Set `reviewEmailedAt` (one email per candidate).

## OpenAI

- Reuse `transcriptionConfig.whisperAPIKey` and OpenAI base URL when configured.
- Model: `gpt-4o-mini` with JSON or plain-text label response.
- Input: talkgroup context, pattern Hz/durations, existing tone set labels, **all N transcripts**.

## Email

- **To:** users with `systemAdmin = true`.
- **Attachments:** `call-{id}.mp3` per call (transcode via ffmpeg if needed).
- Oversized attachments: attach what fits; include admin download links for remainder.

## Admin UI

- **System:** master toggle + duration/tolerance/`callsRequired` settings.
- **Talkgroup:** per-channel toggle (disabled when system master is off).

## Related feature: alerting talkgroup

Per-talkgroup toggle `alertingTalkgroup`:

- Always queues transcription when at least one user has **alerts enabled** on that talkgroup.
- After transcription, sends **transcript** alerts to subscribed users — no tone or keyword matching.
- Bypasses global minimum call duration (like tone-enabled dispatch channels).
- Skips keyword processing and tone alert paths when this flag is on.
