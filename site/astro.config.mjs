import { defineConfig } from 'astro/config';

export default defineConfig({
  site: 'https://aether.fs0ciety.org',
  output: 'static',
  compressHTML: true,
  build: { inlineStylesheets: 'always' },
  image: { responsiveStyles: false },
  devToolbar: { enabled: false },
});
