// Pre-fetch the native binary at install time. If it fails (offline, CI with
// --ignore-scripts, etc.), the launcher downloads it lazily on first run, so this
// must never hard-fail the install.
require('./lib/install')
  .ensure()
  .catch((err) => {
    console.warn(`soul-forge: binary will be downloaded on first run (${err.message})`);
  });
