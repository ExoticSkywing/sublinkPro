English | [简体中文](distribution.zh-CN.md)

# Subscription distribution (production branch)

This fork adds a separate delivery and renewal business. Existing airports, resource subscriptions and `/c/` shares remain unchanged. New links use `/d/<token>` and cannot bypass policy through legacy share endpoints.

## Configure and issue

1. Prepare a resource subscription in the existing subscription manager.
2. Open `/admin/distribution` → Settings. Set the subscription domain and the full public portal URL. The portal defaults to `/`; administrator login is `/admin/login`. If Web Base Path is configured, UI paths retain that prefix; `/api/` and `/d/` do not.
3. Provide a working GeoLite2-City database; a country-only database cannot authorize cities. Optional ip2region and ipdata city fallback is described in [configuration](../configuration.md#distribution-city-fallback-production-fork). Forward real IPs and trust only known proxies. Disable proxy caching for delivery/public APIs and redact tokens from proxy access logs.
4. Issue individual or batch links against the resource subscription, then export them to your delivery system. An explicit batch ID makes retries idempotent. Maximum 100 items per batch.
5. Recipients import the link and configure daily refresh (1440 minutes) with direct access to the subscription domain. The server advertises 24 hours; clients decide whether to honor this header.

Batch-issued links and paid cards use the name as a prefix with a three-digit suffix, such as `September-001` through `September-100`. Single-item issuance keeps the entered name. Existing names are not migrated, and batch retries return the original records. The subscription list shows the name and ID; the full batch ID remains available for inspection and manual copying in **Manage**.

Administrators can inspect, export, disable, re-enable, delete or rotate links. Rotation changes the token, preserving recipient ID, benefits and cities. Each issued link snapshots the configured trial duration; changing the default does not affect previously issued links.

Each row offers **Delete**, with **Delete selected** for batches; both require confirmation. Deletion reuses irreversible revocation (API state remains `revoked`): the link stops working permanently and cannot be restored, edited, rotated or renewed. The default **Not deleted** list hides these rows; filter **Deleted** to inspect read-only details and visits, without old-link copy/export. Redemption references remain clickable and show a Deleted label. Paid codes retain their original binding and cannot be reused. Batch deletion is atomic, rejects any missing ID, and is safe to repeat. Issuance retries for a batch containing deleted links return a conflict; use a new batch. Disabling remains reversible and is not deletion.

Subscription names support inline editing: select the name or pencil, then **Save** (Enter) or **Cancel** (Escape). Leaving the field does not auto-save. Failed saves retain the input and show an error. Quick save patches only the name, preserving the link, resource, expiry and cities; revoked links cannot be renamed. Other settings remain under **Manage**.

To locate a recipient, paste their complete `/d/<token>` link into **Search link, name or batch**. Links with format parameters such as `?client=mihomo`, fragments and surrounding whitespace work, as does the complete 64-character token. Link/token searches match the indexed token digest exactly, never another recipient whose name contains that token. Names and batch IDs still support text search. Existing status filters remain in effect: select **Deleted** to find a deleted link's read-only record. A rotated-out link cannot identify a record because old tokens are not retained; use its current link or name/batch. Searching does not activate, renew or register a city. The UI sends nonempty credential searches via authenticated POST bodies, not URL query strings; do not enable request-body logging for these routes.

Each list row offers **Copy link** without opening details. The button briefly shows **Copied** on success; only a blocked clipboard opens a manual-copy dialog with the link selected. **Manage** still offers **Copy subscription link**, and saving shows an explicit success or error message next to the actions. Resource subscriptions can be changed before or after activation, except for revoked links. The next refresh uses the new resource pool without changing the link, trial start, expiry, benefits or allowed cities. Invalid/deleted resources are rejected; an in-flight response rendered from the previous pool is discarded if the pool changes before final authorization. Status badges use icons and tinted backgrounds as well as labels.

**Copy link** defaults to automatic detection (no `client` query). The adjacent **Format** menu copies an auto-detect, Clash, Mihomo, Surge or V2Ray link in one selection. Explicit formats add `?client=clash`, `mihomo`, `surge` or `v2ray` to the same `/d/<token>` link; they do not issue another credential or bypass UA, activation, city, expiry or disabled/deleted-state checks. Batch exports remain auto-detect links. Manual-copy fallback retains the selected format.

Each card row offers **Copy code** directly, with success feedback and a selected manual-copy field if the clipboard is blocked. Card issuance, current-free-code and export dialogs show pure codes, one per line; **Copy all codes** copies only those codes. **Download text** retains the name and code separated by a tab. Link export keeps its existing name-plus-link format. If clipboard writing is blocked, the dialog selects exactly the content intended for copying.

## Refresh and city policy

Locations resolve through a bounded 24-hour IP cache, GeoLite2, an optional IPv4/IPv6 ip2region offline database, then optional server-side ipdata. Only incomplete results fall through; foreign results are not retried until a source permits them. Provider names map to existing GeoNames IDs using the installed City database. Unknown/ambiguous mappings or conflicting known locations fail closed without consuming city slots. Provider timeouts/quotas do not prevent complete local results from working, nor do they grant unlocated clients access. Check real-world city accuracy before deploying; database output is not proof of a person's location.

Activation and city registration happen only after a recognized client performs a valid GET, usable nonempty output is generated, and the final authorization check succeeds. Browsers redirect to the portal. HEAD, rejected UAs, non-mainland-China IPs, missing cities, empty resources and failed conversions never activate.

The default UA allowlist includes Clash Meta for Android's `ClashMetaForAndroid/…`; importing the existing link automatically selects Clash YAML. On upgrade, this identifier is added only when the saved regexp still equals the complete legacy default. Administrator-defined regexps remain unchanged; add this explicit identifier in distribution settings if needed. IP, city, expiry and link-state checks still apply. If a client reports `Unsupported subscription client` as a YAML parsing error, check for `ua_denied` in access records instead of allowing every UA.

The first valid refresh starts a configurable trial (15 days by default, range 1–365) and registers the first city. Under the default limit of 2, a second valid city registers automatically. Cities beyond the configured limit receive unusable notice nodes only. Disabled, revoked, foreign and unlocated requests also receive no real nodes.

Set **Distribution settings → City limit per subscription** to 1 or 2 (default: 2). Use 1 temporarily for testing, then restore 2. Saving takes effect immediately, including final authorization of in-flight responses and city approvals. Lists, details and public status show the current limit. If any undeleted subscription already exceeds a requested lower limit, the entire settings write fails with `region_limit_conflict`: no bindings are removed and no excess access is silently grandfathered. Keep the previous limit in that case. Upgrades and older API writes omitting the field preserve the saved value.

Disabled and other denial notices put the reason, recovery action and portal URL in separate notice nodes instead of one long name. Disabled links say “订阅已停用” / “请联系管理员恢复”; deleted links say “订阅链接已失效” / “请联系管理员”. These responses never include expiry fallback resources and do not change configured expiry messages.

Successful and denied responses use the recipient link's name for the profile title and download filename. Rejection/expiry messages appear only in notice nodes, not as the profile name. Some clients retain the name chosen on first import: if an older version saved a rejection message as the name, rename that local profile once. Refreshing replaces its contents but cannot force every client to rename its local profile.

Distribution links follow the selected resource subscription's **Realtime usage fetching** switch on each refresh. When off, they neither refresh airport usage nor send `subscription-userinfo`, even if cached usage exists. When on, they use the existing airport-usage refresh and aggregation logic and return usage metadata; airports must have usage fetching enabled and supply usage data. The traffic figures describe the shared resource pool, not a recipient-specific quota. The expiry field uses the recipient link's own expiry, never the airport's; permanent recipients omit expiry. Turning the switch off also hides native client expiry metadata; the public portal still shows the recipient's expiry. Browser/HEAD probes and denial/expiry notices (including fallback nodes) neither refresh nor expose usage. Existing `/c/` share behavior is unchanged. Switch changes apply on the next client refresh; clients that retain old metadata may need it cleared locally.

IP cities and UA are sharing-risk heuristics, not identity authentication: same-city sharing, spoofed UAs and first-use theft remain possible, and carrier geolocation can be wrong. Multiple devices and network switching are allowed. There is currently no physical-device count limit; neither UA count nor IP count is a device count. Previously downloaded node credentials are outside this feature's scope.

After a denied refresh in a new city, recipients open the portal, enter their link, check status, select a denial from the past 7 days and provide a reason (contact optional). Each request targets one city. A link can have one pending request and at most three new requests per 24 hours. At the configured limit, approval must replace an existing city without exceeding that limit. Rejection requires a reply. Recipients check the result on the portal without Telegram login.

The admin notification bell and city-request tab show the pending count. The bell links directly to pending reviews. Counts refresh on entering administration, returning to the window, every 30 seconds while visible, and after reviews or recipient deletion. Marking notifications read or clearing history does not dismiss pending work. A failed refresh shows a retry notice instead of treating an unknown count as zero. These are authenticated in-app reminders, not external push notifications. After a public status lookup, status and the city limit refresh on returning to the window and every 60 seconds while visible. Reload pages left open from an older release once after upgrading.

## Expiry and renewal

| Benefit | Rule |
| --- | --- |
| Trial | Starts on the first valid refresh; 15 days by default |
| Free code | New cycle every 7 days by default; grants 7 days from redemption, shared across recipients, once per link per cycle |
| Three months | Adds 3 calendar months to the later of current expiry and redemption time |
| One year | Adds 1 calendar year using the same rule |
| Permanent | No benefit expiry |

Calendar additions clamp to the last day of the target month. Free renewals do not stack or shorten active trial/paid benefits. A paid code is permanently bound to one subscription ID; repeating redemption returns the original record without extending again. Activate before redeeming. A card's redemption deadline is distinct from the duration it grants.

The public portal distinguishes a new benefit from a repeated redemption. Reusing a free cycle (including a replacement code) or a paid code shows the original redemption time and “No additional renewal was applied”, not a new renewal success. The portal fetches current status afterwards. If that lookup fails, it hides stale status and offers a query retry without misreporting the completed operation as failed.

Redemption history emphasizes subscription and code names, with IDs as secondary labels. Both names open the corresponding management dialog. Names reflect current administrative renames; benefit type and before/after expiry remain historical facts. Deleted codes retain their name and a Deleted label, opening read-only details without the old code or restore actions. Missing related records fall back to a type-and-ID label without removing the redemption record.

New paid and free codes contain 16 cryptographically random letters/digits (80 bits), displayed in four groups such as `K7MP-9XRT-4WQN-8HDC`. Redemption accepts either case, with or without hyphens and ASCII whitespace. Existing 64-character codes remain valid and are not rewritten, including a previously issued current free code and batch retries. Only newly issued cards and new free cycles use the shorter format. Subscription link tokens remain unchanged.

“Get current free code” derives the cycle from its UTC anchor, creates the code on first request and returns it on subsequent requests. No scheduler is required: an external system can call once per cycle. Disabling the current free code does not create a replacement. After deletion, ordinary retrieval cannot restore or silently replace it: the UI asks for explicit reissue confirmation. Reissue generates a new code while preserving once-per-subscription-per-cycle deduplication. Repeated reissue returns the current record, preserving its disabled state if applicable. The next cycle generates normally. Changing the anchor or cycle length changes boundaries and codes; avoid casual changes during operation.

Card rows offer copy, edit, enable/disable, export and delete directly, with enabled-state filters and selected batch deletion. Unused paid codes allow editing name, benefit type (quarter/year/permanent) and redemption deadline without changing the code. Redeemed paid codes allow name/deadline edits, but their benefit type and ownership remain locked. Correct already-granted benefits in the corresponding subscription’s **Manage** dialog by adjusting expiry. Free codes allow name edits, not conversion to paid codes or individual cycle-deadline edits.

Deletion requires confirmation, immediately invalidates the code and removes it from lists. It cannot be reversed through enable or original-batch retries. Database tombstones, ownership and redemption history remain, without revoking granted benefits. Batch deletion is atomic: any nonexistent ID rejects the entire batch. Issuance retries against a batch containing deleted codes return a conflict; use a new batch ID. Unlike deletion, disabling is reversible.

Expired links can return a selected fallback subscription's nodes or notice nodes only, including the renewal URL. Fallback applies only after IP/city checks; cities beyond the limit cannot obtain it. Only fallback nodes are used, not its templates or remote includes. Redeem the original link and code on the public portal, then refresh in the client. The link stays unchanged.

The card list accepts a complete code for exact lookup, alongside name/batch text search, status filters and pagination. Short codes accept either case, optional hyphens and ASCII whitespace; legacy 64-character codes remain searchable. Code fragments do not search code contents. Complete codes use the indexed hash without fuzzy name matches, scanning all encrypted codes or redeeming anything. Deleted cards stay hidden. The UI sends keywords in the authenticated POST `/api/v1/distribution/cards/search` body, never a query URL; do not enable request-body logging.

Under **Settings → Expiry notice nodes**, add, remove and reorder 1–10 messages, each up to 200 characters with no line breaks or control characters. Each message becomes one unusable notice node, followed by any fallback nodes. URLs are not automatically appended to each message. `{portal}` expands to the full public portal URL; for example, use separate messages for “Subscription expired”, “Redeem a code to renew” and “{portal}”. Keep messages short because clients may still truncate long names; repeated names receive a numeric suffix. Saved changes apply on the next client refresh; renewal removes the notices. Other denial messages are unaffected.

Upgrades preserve the legacy wording and move its previously appended URL into a separate node. Saving the list switches to the configured messages. The legacy `expired_message` field remains compatible alongside the new `expired_messages` array; older API writes omitting the new field do not erase an existing list.

## Clients and direct access

Clash, Mihomo and Surge use native rendering. Loon and Shadowrocket automatic detection returns a Base64-encoded native URI list (`v2ray` output, also accepted by these clients), without Sub-Store. Loon therefore follows the working legacy auto-share format. Explicit `?client=loon` still requests a converted Loon configuration; this and expanded formats such as Stash, Quantumult X and sing-box require the existing Sub-Store sidecar setup. Only valid resource output activates. Format selection cannot add support for a node protocol the receiving client does not support.

When a subscription domain is set, Clash/Mihomo/Surge receive a priority DIRECT rule. Other formats may be node lists only; recipients must configure direct access manually, including before the first import. Surge/Surfboard managed-update URLs remain on the distribution link, never a legacy template share or an external renewal website.

## APIs and operations

See the [API reference](../../skill-sublinkpro/reference/api.md#subscription-distribution-production-fork). Management APIs use existing `X-API-Key` or administrator JWT authentication. Public requests never attach the administrator token. No new YAML keys are required; settings reside in separate `distribution_*` tables.

Per-instance IP limits are 30 public API requests/minute and 120 refreshes/minute; excess requests return 429. The latest 200 visits per link are retained; the detail dialog shows 20. Redemption and review records remain. Public status omits raw IPs, UAs and administrator credentials.

Links and codes are secrets, encrypted for administrator export with the existing `SUBLINK_API_ENCRYPTION_KEY`. Back up the complete database and the effective encryption key. Environment-provided keys are not preserved by a database-only backup. The legacy importer cannot migrate distribution data and refuses before clearing data if either source or target contains distribution records. Use full-database backup/restore instead. Stop SQLite or take a consistent snapshot; use native backup tools for external MySQL/PostgreSQL.

Use Vite + Air for development, validate the UI, then deploy production separately. Never mount development data into production. Back up before rollback; older binaries cannot serve the new distribution routes.
