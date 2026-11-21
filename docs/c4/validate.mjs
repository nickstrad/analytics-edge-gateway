// Decodes model.json with c4-sdk's decodeArchitecture and builds every scene.
// Exits 1 on any error diagnostic. Run from a directory where c4-sdk is installed:
//   mkdir -p /tmp/c4-validate && cd /tmp/c4-validate && npm init -y >/dev/null && npm install c4-sdk@0.1.0
//   node /path/to/repo/docs/c4/validate.mjs
import { readFileSync } from 'node:fs'
import { createRequire } from 'node:module'
import { fileURLToPath } from 'node:url'

const { decodeArchitecture, buildScene } = createRequire(process.cwd() + '/')('c4-sdk/core')
const file = fileURLToPath(new URL('model.json', import.meta.url))
const decoded = decodeArchitecture(JSON.parse(readFileSync(file, 'utf8')))
for (const d of decoded.diagnostics) console.log(`${d.severity} ${d.code} ${d.path} ${d.message}`)
if (!decoded.ok) { console.log('decodeArchitecture: FAILED'); process.exit(1) }
const { bundle, index } = decoded.value
let failed = false
for (const diagram of bundle.diagrams) {
  const scene = buildScene(index, diagram.id)
  if (!scene.ok) { failed = true; for (const d of scene.diagnostics) console.log(`${diagram.id}: ${d.code} ${d.message}`) }
}
console.log(`decodeArchitecture: ok, schemaVersion ${bundle.schemaVersion}, ${bundle.model.elements.length} elements, ${bundle.model.relationships.length} relationships, ${bundle.diagrams.length} diagrams, ${decoded.diagnostics.length} diagnostics; buildScene: ${failed ? 'FAILED' : 'ok for every diagram'}`)
process.exit(failed ? 1 : 0)
