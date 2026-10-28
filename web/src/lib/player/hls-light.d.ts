// hls.js ships no typings for its "light" entry point: it shares the main build's API.
declare module 'hls.js/light' {
	import Hls from 'hls.js';
	export * from 'hls.js';
	export default Hls;
}
