// Shared UI state between pages and the app shell.
export const shell = $state({
	/** True while the top of the page is a full-bleed image (the header then uses on-image ink). */
	overImage: false
});
