import assert from 'node:assert/strict';
import { existsSync, readFileSync } from 'node:fs';

const read = (file) => readFileSync(new URL(`../${file}`, import.meta.url), 'utf8').replaceAll('\r\n', '\n');
const fields = [
  ['AGENTS.md', /JackNiu2026\/ccbox/],
  ['Makefile', /^all: cage$/m],
  ['Makefile', /^install:\s*\n\t@.*ccbox\.org.*\n\t@exit 1$/m],
  ['Makefile', /\.\/cmd\/cage$/m],
  ['scripts/build-app.sh', /^app_name=CCBox$/m],
  ['scripts/build-app.sh', /^bundle_id=org\.ccbox\.desktop$/m],
  ['desktop/windows/Cage.Windows.csproj', /<Product>CCBox<\/Product>/],
  ['packaging/windows/Cage.nsi', /^VIAddVersionKey \/LANG=1033 "ProductName" "CCBox"$/m],
  ['packaging/windows/Cage.nsi', /^InstallDir "\$LOCALAPPDATA\\Programs\\CCBox"$/m],
  ['packaging/linux/org.ccbox.desktop.desktop', /^Name=CCBox$/m],
  ['packaging/linux/org.ccbox.desktop.desktop', /^Exec=\/usr\/bin\/ccbox-desktop$/m],
  ['ui/index.html', /<title>CCBox<\/title>/],
  ['ui/index.html', /rel="icon"[^>]+href="\/ccbox\.svg"/],
];
for (const [file, expected] of fields) {
  assert.match(read(file), expected, `${file} does not declare the expected CCBox identity (${expected}).`);
}
assert.ok(!existsSync(new URL('../scripts/upstream-install.sh', import.meta.url)), 'The upstream installer must not ship in the CCBox checkout.');
console.log('Verified CCBox desktop branding and build entry points.');
