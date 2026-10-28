// Sources (M5b): labels, descriptions and the editable draft behind the source forms.
import type { IconName } from '$lib/components/Icon.svelte';
import type { AgentTransport, SftpAuth, Source, SourceInput, SourceKind, SourcePatch, SourceStatus, SourceTestResult } from '$lib/api/types';

export const SOURCE_KINDS: { id: SourceKind; label: string; icon: IconName; hint: string }[] = [
	{ id: 'local', label: 'Dossier local', icon: 'folder', hint: 'Un chemin absolu sur le serveur Aether (disque, NAS monté…).' },
	{ id: 'agent', label: 'Agent Aether', icon: 'server', hint: 'Une machine distante que vous contrôlez, où tourne l’agent Aether.' },
	{ id: 'sftp', label: 'SFTP / seedbox', icon: 'cloud', hint: 'Un serveur SFTP : seedbox, VPS, NAS accessible en SSH.' }
];

export function sourceKindInfo(kind: SourceKind) {
	return SOURCE_KINDS.find((k) => k.id === kind) ?? SOURCE_KINDS[0];
}

export const STATUS_LABEL: Record<SourceStatus, string> = {
	online: 'En ligne',
	offline: 'Hors ligne',
	unknown: 'Inconnu',
	cert_mismatch: 'Certificat changé'
};

/** SHA-256 fingerprint for humans: 16 groups of 4 uppercase hex digits (the agent shows the same). */
export function formatFingerprint(fp: string | undefined): string {
	const hex = (fp ?? '').replace(/[^0-9a-f]/gi, '').toUpperCase();
	if (hex.length !== 64) return fp ?? '';
	return hex.match(/.{4}/g)!.join(' ');
}

/** Copy for how the server reaches an agent (Settings › Agents). */
export const TRANSPORT_LABEL: Record<AgentTransport, string> = {
	'https-pinned': 'HTTPS · certificat épinglé',
	'https-ca': 'HTTPS · autorité reconnue',
	http: 'HTTP non chiffré'
};

/** Secondary line under a source name: where it lives. */
export function sourceLocation(s: Pick<Source, 'kind' | 'root' | 'agent' | 'sftp'>): string {
	if (s.kind === 'agent' && s.agent) {
		const host = s.agent.hostname || hostOf(s.agent.url ?? '') || 'Agent';
		const path = [s.agent.share, s.root].filter(Boolean).join('/');
		return `${host} · ${path || 'racine'}`;
	}
	if (s.kind === 'sftp' && s.sftp) {
		const port = s.sftp.port && s.sftp.port !== 22 ? `:${s.sftp.port}` : '';
		return `${s.sftp.user}@${s.sftp.host}${port}:${s.root || '/'}`;
	}
	return s.root;
}

export function hostOf(url: string): string {
	try {
		return new URL(url).host;
	} catch {
		return url;
	}
}

/** Known machine codes from POST /sources/test (and source.status lastError) → French copy. */
const ERRORS: Record<string, string> = {
	host_key_unknown: 'Empreinte de l’hôte inconnue : confirmez-la pour continuer.',
	host_key_mismatch: 'L’empreinte de l’hôte a changé. Si vous n’avez pas réinstallé le serveur, il peut s’agir d’une usurpation.',
	auth_failed: 'Authentification refusée : vérifiez l’utilisateur et le mot de passe ou la clé.',
	unauthorized: 'Jeton refusé par l’agent : vérifiez le jeton d’appairage.',
	invalid_token: 'Jeton refusé par l’agent : vérifiez le jeton d’appairage.',
	forbidden: 'Accès refusé.',
	rate_limited: 'Trop de tentatives : patientez une minute avant de réessayer.',
	too_many_requests: 'Trop de tentatives : patientez une minute avant de réessayer.',
	connection_refused: 'Connexion refusée : le service n’écoute pas sur ce port.',
	unreachable: 'Hôte injoignable : vérifiez l’adresse et le réseau (VPN, pare-feu).',
	timeout: 'Délai dépassé : l’hôte ne répond pas.',
	dns: 'Nom d’hôte introuvable.',
	not_found: 'Dossier introuvable.',
	not_a_directory: 'Ce chemin n’est pas un dossier.',
	permission_denied: 'Permission refusée sur ce dossier.',
	not_absolute: 'Le chemin doit être absolu.',
	share_not_found: 'Ce partage n’existe pas sur l’agent.',
	invalid_key: 'Clé privée illisible : collez une clé au format PEM / OpenSSH.',
	passphrase_required: 'Cette clé est protégée : indiquez sa phrase de passe.',
	bad_passphrase: 'Phrase de passe incorrecte.',
	tls: 'Échec TLS : certificat invalide ou protocole incorrect (http/https).',
	version_mismatch: 'Version de l’agent incompatible avec ce serveur.',
	cert_mismatch:
		'L’agent présente un autre certificat que celui enregistré à l’appairage : les échanges sont bloqués. Si vous avez réinstallé l’agent ou supprimé son certificat, réappairez-le ; sinon, quelqu’un intercepte peut-être la connexion.',
	agent_cert_mismatch:
		'L’agent présente un autre certificat que celui enregistré à l’appairage : les échanges sont bloqués. Si vous avez réinstallé l’agent ou supprimé son certificat, réappairez-le ; sinon, quelqu’un intercepte peut-être la connexion.',
	tls_required: 'Cet agent exige HTTPS : réappairez-le pour passer en HTTPS.',
	already_covered: 'Ce dossier est déjà couvert par une autre source.'
};

const PATTERNS: [RegExp, string][] = [
	[/rejected the token|invalid token|\b401\b|unauthori[sz]ed/, 'unauthorized'],
	[/host key mismatch|key mismatch|knownhosts: key mismatch/, 'host_key_mismatch'],
	[/unable to authenticate|auth(entication)? failed|permission denied \(publickey|password\)/, 'auth_failed'],
	[/passphrase protected|passphrase is required/, 'passphrase_required'],
	[/decryption password incorrect|bad passphrase/, 'bad_passphrase'],
	[/no key found|cannot decode encrypted|ssh: no key|parse private key/, 'invalid_key'],
	[/connection refused/, 'connection_refused'],
	[/no such host|server misbehaving/, 'dns'],
	[/i\/o timeout|deadline exceeded|timed out|timeout/, 'timeout'],
	[/no route to host|network is unreachable|host is unreachable/, 'unreachable'],
	[/too many requests|\b429\b/, 'rate_limited'],
	[/share.*not found|unknown share/, 'share_not_found'],
	[/no such file|not found|does not exist/, 'not_found'],
	[/not a directory/, 'not_a_directory'],
	[/permission denied/, 'permission_denied'],
	[/certificat de l.agent inattendu|fingerprint mismatch/, 'cert_mismatch'],
	[/exige https|tls_required/, 'tls_required'],
	[/x509|tls:|certificate/, 'tls'],
	[/already covered|déjà couvert/, 'already_covered']
];

/** French message for an error code; unknown codes / free text are shown as-is. */
export function sourceErrorMessage(err: string | undefined): string {
	if (!err) return 'La connexion a échoué.';
	const code = err.trim();
	if (ERRORS[code]) return ERRORS[code];
	const lower = code.toLowerCase();
	for (const [k, v] of Object.entries(ERRORS)) if (lower.startsWith(k + ':') || lower.startsWith(k + ' ')) return v;
	// Free-text errors from the server (Go error chains): recognise the common causes.
	for (const [re, key] of PATTERNS) if (re.test(lower)) return ERRORS[key];
	return `La connexion a échoué (${code}).`;
}

// ——— Draft ———

export type AgentMode = 'paired' | 'token';

export interface SourceDraft {
	kind: SourceKind;
	name: string;
	/** local path · agent sub-folder · sftp remote path */
	root: string;
	agent: {
		/** M5c: `paired` = an agent paired by code (`agentId`) · `token` = manual token (agent.toml). */
		mode: AgentMode;
		agentId?: number;
		/** Display name of the paired agent, and local path of the chosen share (for labels). */
		agentName?: string;
		sharePath?: string;
		url: string;
		token: string;
		share: string;
		/** Manual token over https (M5d): optional certificate fingerprint; empty = pinned on first use. */
		fingerprint?: string;
	};
	sftp: { host: string; port: string; user: string; auth: SftpAuth; password: string; privateKey: string; passphrase: string; hostKey: string };
	/** Shares returned by the last successful agent test. */
	shares: string[];
	/** Result of the last test, and the fingerprint of the input it ran on. */
	test?: SourceTestResult & { key: string };
}

export function emptyDraft(kind: SourceKind): SourceDraft {
	return {
		kind,
		name: '',
		root: '',
		agent: { mode: 'paired', url: '', token: '', share: '' },
		sftp: { host: '', port: '22', user: '', auth: 'password', password: '', privateKey: '', passphrase: '', hostKey: '' },
		shares: []
	};
}

/** Draft for editing an existing source: secrets stay empty (= unchanged). */
export function draftFromSource(s: Source): SourceDraft {
	const d = emptyDraft(s.kind);
	d.name = s.name ?? '';
	d.root = s.root ?? '';
	if (s.agent) {
		d.agent.mode = s.agent.agentId ? 'paired' : 'token';
		d.agent.agentId = s.agent.agentId || undefined;
		d.agent.agentName = s.agent.hostname;
		d.agent.url = s.agent.url ?? '';
		d.agent.fingerprint = s.agent.fingerprint ?? '';
		d.agent.share = s.agent.share;
		d.shares = s.agent.share ? [s.agent.share] : [];
	}
	if (s.sftp) {
		d.sftp.host = s.sftp.host;
		d.sftp.port = String(s.sftp.port || 22);
		d.sftp.user = s.sftp.user;
		d.sftp.auth = s.sftp.auth ?? 'password';
		d.sftp.hostKey = s.sftp.hostKey ?? '';
	}
	return d;
}

export function normalizeAgentUrl(url: string): string {
	let u = url.trim().replace(/\/+$/, '');
	if (u && !/^https?:\/\//i.test(u)) u = `http://${u}`;
	return u;
}

export function draftToInput(d: SourceDraft): SourceInput {
	const name = d.name.trim() || undefined;
	if (d.kind === 'agent') {
		const root = d.root.trim().replace(/^\/+|\/+$/g, '');
		if (d.agent.mode === 'paired') return { kind: 'agent', name, root, agent: { agentId: d.agent.agentId ?? 0, share: d.agent.share } };
		return {
			kind: 'agent',
			name,
			root,
			agent: {
				url: normalizeAgentUrl(d.agent.url),
				token: d.agent.token.trim(),
				share: d.agent.share,
				...(d.agent.fingerprint?.trim() ? { fingerprint: d.agent.fingerprint.trim() } : {})
			}
		};
	}
	if (d.kind === 'sftp') {
		const s = d.sftp;
		return {
			kind: 'sftp',
			name,
			root: d.root.trim(),
			sftp: {
				host: s.host.trim(),
				port: Number(s.port) || 22,
				user: s.user.trim(),
				...(s.auth === 'password' ? { password: s.password } : { privateKey: s.privateKey.trim(), passphrase: s.passphrase || undefined }),
				hostKey: s.hostKey || undefined
			}
		};
	}
	return { kind: 'local', name, root: d.root.trim() };
}

/**
 * Fingerprint of what a connection test depends on. For an agent the share and sub-folder are chosen
 * after the test (from its list), so only the URL and token count.
 */
export function testKey(d: SourceDraft): string {
	const i = draftToInput(d);
	if (i.kind === 'agent' && d.agent.mode === 'paired') return JSON.stringify(['agent', d.agent.agentId, d.agent.share, i.root]);
	if (i.kind === 'agent' && i.agent && 'url' in i.agent) return JSON.stringify(['agent', i.agent.url, i.agent.token, i.agent.fingerprint ?? '']);
	return JSON.stringify(i);
}

export function isTested(d: SourceDraft): boolean {
	return !!d.test?.ok && d.test.key === testKey(d);
}

export function isAbsolutePath(p: string): boolean {
	return p.startsWith('/') || /^[A-Za-z]:[\\/]/.test(p);
}

export type DraftErrors = Partial<Record<'name' | 'root' | 'url' | 'token' | 'share' | 'host' | 'port' | 'user' | 'password' | 'privateKey', string>>;

/**
 * Field validation. `editing`: secrets may be left empty (unchanged on the server).
 * `forTest`: agent share is not required yet (it comes from the test).
 */
export function validateDraft(d: SourceDraft, { editing = false, forTest = false } = {}): DraftErrors {
	const e: DraftErrors = {};
	const root = d.root.trim();
	if (d.kind === 'local') {
		if (!root) e.root = 'Indiquez le chemin du dossier.';
		else if (!isAbsolutePath(root)) e.root = 'Le chemin doit être absolu (ex. /srv/media/films).';
	} else if (d.kind === 'agent' && d.agent.mode === 'paired') {
		if (!d.agent.agentId) e.share = 'Choisissez ou appairez un agent.';
		else if (!d.agent.share) e.share = 'Choisissez le dossier à partager.';
		if (root.split('/').includes('..')) e.root = 'Le sous-dossier ne peut pas contenir « .. ».';
	} else if (d.kind === 'agent') {
		const url = normalizeAgentUrl(d.agent.url);
		if (!url) e.url = 'Indiquez l’adresse de l’agent.';
		else {
			try {
				const u = new URL(url);
				if (!u.hostname) throw new Error();
			} catch {
				e.url = 'Adresse invalide (ex. https://nas.local:8097).';
			}
		}
		if (!editing && !d.agent.token.trim()) e.token = 'Collez le jeton d’appairage de l’agent.';
		if (!forTest && !d.agent.share) e.share = 'Choisissez un partage.';
		if (root.split('/').includes('..')) e.root = 'Le sous-dossier ne peut pas contenir « .. ».';
	} else {
		const s = d.sftp;
		if (!s.host.trim()) e.host = 'Indiquez l’hôte.';
		else if (/\s|\//.test(s.host.trim())) e.host = 'Hôte invalide : nom ou adresse IP, sans « sftp:// ».';
		const port = Number(s.port);
		if (!Number.isInteger(port) || port < 1 || port > 65535) e.port = 'Port entre 1 et 65535.';
		if (!s.user.trim()) e.user = 'Indiquez l’utilisateur.';
		if (!editing && s.auth === 'password' && !s.password) e.password = 'Indiquez le mot de passe.';
		if (!editing && s.auth === 'key' && !s.privateKey.trim()) e.privateKey = 'Collez la clé privée.';
		if (s.auth === 'key' && s.privateKey.trim() && !/-----BEGIN [A-Z ]*PRIVATE KEY-----/.test(s.privateKey))
			e.privateKey = 'Format attendu : PEM ou OpenSSH (« -----BEGIN … PRIVATE KEY----- »).';
		if (!root) e.root = 'Indiquez le dossier distant.';
		else if (!root.startsWith('/')) e.root = 'Le dossier distant doit être absolu (ex. /home/moi/downloads).';
	}
	return e;
}

/** Only the changed fields; empty secrets are omitted (unchanged). */
export function draftToPatch(d: SourceDraft, orig: Source): SourcePatch {
	const i = draftToInput(d);
	const p: SourcePatch = {};
	if ((i.name ?? '') !== (orig.name ?? '') && i.name !== undefined) p.name = i.name;
	if (i.root !== orig.root) p.root = i.root;
	if (i.kind === 'agent' && i.agent && 'agentId' in i.agent) {
		if (i.agent.share !== orig.agent?.share) p.agent = { agentId: i.agent.agentId, share: i.agent.share };
		// The server rebuilds a PATCH from the stored settings without `agentId`: always resend it,
		// or a simple rename would be read as a manual-token source without URL.
		if (Object.keys(p).length) p.agent = { ...p.agent, agentId: i.agent.agentId };
	} else if (i.kind === 'agent' && i.agent && 'url' in i.agent) {
		const a: SourcePatch['agent'] = {};
		if (i.agent.url !== orig.agent?.url) a.url = i.agent.url;
		if (i.agent.share !== orig.agent?.share) a.share = i.agent.share;
		if (i.agent.token) a.token = i.agent.token;
		if (i.agent.fingerprint && i.agent.fingerprint !== orig.agent?.fingerprint) a.fingerprint = i.agent.fingerprint;
		if (Object.keys(a).length) p.agent = a;
	}
	if (i.kind === 'sftp' && i.sftp) {
		const o = orig.sftp;
		const s: NonNullable<SourcePatch['sftp']> = {};
		if (i.sftp.host !== o?.host) s.host = i.sftp.host;
		if (i.sftp.port !== o?.port) s.port = i.sftp.port;
		if (i.sftp.user !== o?.user) s.user = i.sftp.user;
		if (i.sftp.password) s.password = i.sftp.password;
		if (i.sftp.privateKey) s.privateKey = i.sftp.privateKey;
		if (i.sftp.passphrase) s.passphrase = i.sftp.passphrase;
		if (i.sftp.hostKey && i.sftp.hostKey !== o?.hostKey) s.hostKey = i.sftp.hostKey;
		if (Object.keys(s).length) p.sftp = s;
	}
	return p;
}

/** Editing: does the change touch the connection (so it must be tested again)? */
export function connectionChanged(d: SourceDraft, orig: Source): boolean {
	const p = draftToPatch(d, orig);
	if (d.kind === 'agent' && d.agent.mode === 'paired') return !!p.agent?.share;
	if (d.kind === 'agent') return !!(p.agent?.url || p.agent?.token);
	if (d.kind === 'sftp') return !!p.sftp;
	return false;
}

/** Short label for a draft in the add-library list. */
export function draftLabel(d: SourceDraft): { title: string; detail: string } {
	const i = draftToInput(d);
	if (i.kind === 'agent') {
		const url = i.agent && 'url' in i.agent ? i.agent.url : '';
		const host = d.agent.agentName || d.test?.agent?.hostname || hostOf(url);
		const where = d.agent.sharePath || [i.agent?.share, i.root].filter(Boolean).join('/') || 'racine';
		return { title: i.name || `${host} · ${i.agent?.share}`, detail: `Agent · ${where}` };
	}
	if (i.kind === 'sftp') return { title: i.name || `${i.sftp?.host}`, detail: `SFTP · ${i.sftp?.user}@${i.sftp?.host}:${i.root}` };
	return { title: i.name || i.root, detail: 'Dossier local' };
}

// ——— Agents appairés (M5c) ———

/** Pairing code alphabet (no ambiguous characters), shown as XXXX-XXXX. */
export const PAIR_ALPHABET = 'ABCDEFGHJKMNPQRSTUVWXYZ23456789';

/** Uppercases, drops foreign characters and inserts the dash: "k7pq4x" → "K7PQ-4X". */
export function formatPairCode(raw: string): string {
	const c = raw
		.toUpperCase()
		.replace(/[^A-Z0-9]/g, '')
		.slice(0, 8);
	return c.length > 4 ? `${c.slice(0, 4)}-${c.slice(4)}` : c;
}

/** Problem with a typed code, or null when it looks valid. */
export function pairCodeProblem(code: string): string | null {
	const c = code.replace(/-/g, '');
	if (!c) return 'Saisissez le code affiché par l’agent.';
	if (c.length < 8) return 'Le code fait 8 caractères (XXXX-XXXX).';
	const bad = [...c].find((ch) => !PAIR_ALPHABET.includes(ch));
	if (bad) return `Caractère « ${bad} » impossible : le code n’utilise ni O, ni 0, ni I, ni 1, ni L.`;
	return null;
}

/** Validated agent address ("192.168.1.20:8097" → "http://192.168.1.20:8097"), or an error. */
export function parseAgentAddress(raw: string): { url: string } | { error: string } {
	let u = normalizeAgentUrl(raw);
	if (!u) return { error: 'Indiquez l’adresse de l’agent.' };
	try {
		const parsed = new URL(u);
		if (!parsed.hostname || (parsed.pathname !== '/' && parsed.pathname !== '')) throw new Error();
		if (!parsed.port && parsed.protocol === 'http:' && !/:\d+$/.test(raw.trim().replace(/\/+$/, ''))) parsed.port = '8097';
		u = parsed.origin;
	} catch {
		return { error: 'Adresse invalide (ex. 192.168.1.20 ou https://nas.local:8097).' };
	}
	return { url: u };
}

/** Error of an agent call (pair, browse, shares) → French copy. */
export function agentErrorMessage(err: unknown, action: 'pair' | 'browse' | 'share' = 'browse'): string {
	if (err instanceof TypeError) return 'Le serveur Aether est injoignable.';
	const e = err as { status?: number; code?: string; message?: string };
	const code = e?.code ?? '';
	const msg = (e?.message ?? '').trim();
	if (action === 'pair') {
		if (e?.status === 400 && msg) return sentenceCase(msg);
		// The server answers 500 when it cannot reach the agent during pairing.
		if (e?.status === 500 || e?.status === 502 || code === 'agent_offline')
			return 'Agent injoignable à cette adresse : vérifiez qu’il est lancé, l’adresse et le port (8097 par défaut), et le pare-feu.';
	}
	if (code === 'agent_unpaired') return 'L’agent ne reconnaît plus ce serveur (appairage révoqué) : appairez-le de nouveau avec un code.';
	if (code === 'agent_cert_mismatch') return ERRORS.cert_mismatch;
	if (code === 'agent_tls_required') return ERRORS.tls_required;
	if (code === 'agent_offline' || e?.status === 502) return 'Agent injoignable : vérifiez que la machine est allumée et que l’agent tourne.';
	if (code === 'denied' || e?.status === 403) return 'Dossier hors des emplacements autorisés sur cette machine.';
	if (code === 'not_found' || e?.status === 404) return action === 'share' ? 'Ce dossier n’existe plus.' : 'Dossier introuvable ou illisible.';
	if (msg) return sentenceCase(msg);
	return 'La requête a échoué.';
}

function sentenceCase(m: string): string {
	return m.charAt(0).toUpperCase() + m.slice(1) + (/[.!?…]$/.test(m) ? '' : '.');
}
