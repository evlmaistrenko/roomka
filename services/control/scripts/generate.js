/**
 * Regenerates the control server's Go code from its specs, but only the parts whose spec actually
 * changed.
 *
 * Each spec (the GraphQL SDL, the OpenAPI document) is hashed and compared with the hash recorded after
 * its last successful generation. Editors report one save as several events, and a save need not change
 * anything, so a file event only asks for this check; it never generates by itself.
 *
 *     node services/control/scripts/generate.js          check once and exit (dev:control, before air)
 *     node services/control/scripts/generate.js --watch  check on every spec save (dev:control-specs)
 *
 * The two are split so the startup check finishes before air's first build, instead of racing it.
 */
import { spawn } from "node:child_process"
import { createHash } from "node:crypto"
import { watch } from "node:fs"
import { mkdir, readFile, readdir, rm, writeFile } from "node:fs/promises"
import { basename, join } from "node:path"

const SERVICE_DIRECTORY = join(import.meta.dirname, "..")
const REPOSITORY_DIRECTORY = join(SERVICE_DIRECTORY, "..", "..")
const SPECS_DIRECTORY = join(REPOSITORY_DIRECTORY, "packages", "control-specs")
// The recorded hashes are also the signal air rebuilds on (see ../.air.toml): each is written only
// once its run has finished, so air never builds a package a generator left half-written.
const HASH_DIRECTORY = join(REPOSITORY_DIRECTORY, ".runtime", "control")
const SETTLE_MILLISECONDS = 200

// A spec, the files it consists of, and the Go package generated from it.
const TARGETS = [
	{
		name: "graphql",
		files: () => filesIn(join(SPECS_DIRECTORY, "graphql"), ".graphqls"),
		goPackage: "./graph",
	},
	{
		name: "openapi",
		files: async () => [join(SPECS_DIRECTORY, "openapi.yaml")],
		goPackage: "./rest",
	},
]

async function filesIn(directory, extension) {
	return (await readdir(directory))
		.filter((name) => name.endsWith(extension))
		.sort()
		.map((name) => join(directory, name))
}

async function hashTarget(target) {
	const hash = createHash("sha256")
	for (const file of await target.files()) {
		// The name is hashed too, so a rename or a new empty file counts as a change.
		hash.update(basename(file)).update("\0")
		hash.update(await readFile(file)).update("\0")
	}
	return hash.digest("hex")
}

const hashFile = (target) => join(HASH_DIRECTORY, `${target.name}.sha256`)

// Read from the file every time rather than kept in memory, so a one-off run and a running watcher
// never disagree about what was generated last.
async function readGeneratedHash(target) {
	try {
		return (await readFile(hashFile(target), "utf8")).trim()
	} catch {
		return undefined
	}
}

// A failed run may still have rewritten some generated files, so after one nothing is known to be
// generated: the hash is removed, and the next check regenerates whatever the spec holds, even a
// revert to the last good state.
async function recordGeneratedHash(target, hash) {
	if (hash === undefined) await rm(hashFile(target), { force: true })
	else await writeFile(hashFile(target), hash + "\n")
}

function generate(target) {
	return new Promise((resolve) => {
		const child = spawn("go", ["generate", target.goPackage], {
			cwd: SERVICE_DIRECTORY,
			stdio: "inherit",
		})
		child.on("exit", (code) => resolve(code === 0))
	})
}

let running = false
let pending = false

// One pass at a time, returning whether every spec ended up generated. A change that arrives mid-run
// sets pending, and the loop checks again once the run ends, so the last save is never lost and never
// generated twice.
async function regenerateChanged() {
	if (running) {
		pending = true
		return true
	}
	running = true
	let succeeded = true
	do {
		pending = false
		succeeded = true
		for (const target of TARGETS) {
			const hash = await hashTarget(target)
			if (hash === (await readGeneratedHash(target))) continue
			console.log(`[control-specs] ${target.name} changed, regenerating`)
			const generated = await generate(target)
			await recordGeneratedHash(target, generated ? hash : undefined)
			succeeded &&= generated
		}
	} while (pending)
	running = false
	return succeeded
}

// Created up front, even when nothing gets generated: air watches this directory only if it exists
// when air starts.
await mkdir(HASH_DIRECTORY, { recursive: true })

if (process.argv.includes("--watch")) {
	let settleTimer
	// Any event re-checks every spec; hashing them is cheap next to telling events apart.
	watch(SPECS_DIRECTORY, { recursive: true }, () => {
		clearTimeout(settleTimer)
		settleTimer = setTimeout(regenerateChanged, SETTLE_MILLISECONDS)
	})
	console.log(`[control-specs] watching ${SPECS_DIRECTORY}`)
} else if (!(await regenerateChanged())) {
	process.exitCode = 1
}
