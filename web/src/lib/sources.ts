// Sources (M5b): labels, descriptions and the editable draft behind the source forms.
import type { IconName } from '$lib/components/Icon.svelte';
import type { SftpAuth, Source, SourceInput, SourceKind, SourcePatch, SourceStatus, SourceTestResult } from '$lib/api/types';

export const SOURCE_KINDS: { id: SourceKind; label: string; icon: IconName; hint: string }[] = [
	{ id: 'local', label: 'Dossier local', icon: 'folder', hint: 'Un chemin absolu sur le serveur Aether (disque, NAS monté…).' },
	{ id: 'agent', label: 'Agent Aether', icon: 'server', hint: 'Une machine distante que vous contrôlez, où tourne l’agent Aether.' },
	{ id: 'sftp', label: 'SFTP / seedbox', icon: 'cloud', hint: 'Un serveur SFTP : seedbox, VPS, NAS accessible en SSH.' }
];

export function sourceKindInfo(kind: SourceKind) {
	return SOURCE_KINDS.find((k) => k.id === kind) ?? SOURCE_KINDS[0];
}

export const STATUS_LABEL: Record<SourceStatus, string> = { online: 'En ligne', offline: 'Hors ligne', unknown: 'Inconnu' };

/** Secondary line under a source name: where it lives. */
export function sourceLocation(s: Pick<Source, 'kind' | 'root' | 'agent' | 'sftp'>): string {
	if (s.kind === 'agent' && s.agent) {
		const host = s.agent.hostname || hostOf(s.agent.url);
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

export interface SourceDraft {
	kind: SourceKind;
	name: string;
	/** local path · agent sub-folder · sftp remote path */
	root: string;
	agent: { url: string; token: string; share: string };
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
		agent: { url: '', token: '', share: '' },
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
		d.agent.url = s.agent.url;
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
		return {
			kind: 'agent',
			name,
			root: d.root.trim().replace(/^\/+|\/+$/g, ''),
			agent: { url: normalizeAgentUrl(d.agent.url), token: d.agent.token.trim(), share: d.agent.share }
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
	if (i.kind === 'agent') return JSON.stringify(['agent', i.agent?.url, i.agent?.token]);
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
	} else if (d.kind === 'agent') {
		const url = normalizeAgentUrl(d.agent.url);
		if (!url) e.url = 'Indiquez l’adresse de l’agent.';
		else {
			try {
				const u = new URL(url);
				if (!u.hostname) throw new Error();
			} catch {
				e.url = 'Adresse invalide (ex. http://nas.local:8097).';
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
	if (i.kind === 'agent' && i.agent) {
		const a: SourcePatch['agent'] = {};
		if (i.agent.url !== orig.agent?.url) a.url = i.agent.url;
		if (i.agent.share !== orig.agent?.share) a.share = i.agent.share;
		if (i.agent.token) a.token = i.agent.token;
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
	if (d.kind === 'agent') return !!(p.agent?.url || p.agent?.token);
	if (d.kind === 'sftp') return !!p.sftp;
	return false;
}

/** Short label for a draft in the add-library list. */
export function draftLabel(d: SourceDraft): { title: string; detail: string } {
	const i = draftToInput(d);
	if (i.kind === 'agent') {
		const host = d.test?.agent?.hostname || hostOf(i.agent?.url ?? '');
		return { title: i.name || `${host} · ${i.agent?.share}`, detail: `Agent · ${[i.agent?.share, i.root].filter(Boolean).join('/') || 'racine'}` };
	}
	if (i.kind === 'sftp') return { title: i.name || `${i.sftp?.host}`, detail: `SFTP · ${i.sftp?.user}@${i.sftp?.host}:${i.root}` };
	return { title: i.name || i.root, detail: 'Dossier local' };
}
