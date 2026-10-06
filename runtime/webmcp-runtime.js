/**
 * @webmcp/runtime v0.1.0 — manifest v1
 * Registers server-defined tools and invokes their same-origin HTTP endpoints.
 * The application keeps responsibility for authentication, CSRF protection, and
 * authorization; this runtime uses the existing browser session and endpoints.
 */

const forbiddenKeys = new Set(['__proto__', 'constructor', 'prototype']);
const messages = {
  invalid_input: 'Provide only declared parameters with the required types.',
  csrf_token_missing: 'Refresh the page to obtain a CSRF token, then try again.',
  network_error: 'The request failed. Check the connection and try again.',
  aborted: 'The request was cancelled.',
  invalid_response: 'The endpoint returned invalid JSON. Check the endpoint response.',
  response_too_large: 'Narrow the request and try again.',
  unknown_outcome: 'The result is unknown. Do not retry automatically; ask the user to check.',
};

function failure(code, status) {
  return {
    ok: false,
    ...(status === undefined ? {} : { status }),
    error: {
      code,
      message: code === 'http_error'
        ? `The endpoint returned HTTP ${status}. Check the request and your access.`
        : messages[code],
    },
  };
}

function matchesScalar(type, value) {
  switch (type) {
    case 'string': return typeof value === 'string';
    case 'boolean': return typeof value === 'boolean';
    case 'number': return typeof value === 'number' && Number.isFinite(value);
    case 'integer': return Number.isInteger(value);
    default: return false;
  }
}

/** Returns { ok: true } or an invalid_input outcome. No schema constraint checks. */
export function validateInput(tool, input) {
  if (input === null || typeof input !== 'object' || Array.isArray(input)) {
    return failure('invalid_input');
  }
  const schema = tool.inputSchema ?? {};
  const properties = schema.properties ?? {};
  for (const key of schema.required ?? []) {
    if (!Object.hasOwn(input, key)) return failure('invalid_input');
  }
  for (const key of Object.keys(input)) {
    if (forbiddenKeys.has(key) || !Object.hasOwn(properties, key)) {
      return failure('invalid_input');
    }
    const property = properties[key];
    const value = input[key];
    if (property.type === 'array') {
      if (!Array.isArray(value)) return failure('invalid_input');
      // Iteration also rejects sparse slots, which would serialize as null.
      for (const item of value) {
        if (!matchesScalar(property.items?.type, item)) return failure('invalid_input');
      }
    } else if (!matchesScalar(property.type, value)) {
      return failure('invalid_input');
    }
  }
  return { ok: true };
}

function* parameters(tool, input) {
  const properties = tool.inputSchema?.properties ?? {};
  const mapping = tool.endpoint.paramMap ?? {};
  for (const key of Object.keys(input)) {
    if (!forbiddenKeys.has(key) && Object.hasOwn(properties, key)) {
      yield [Object.hasOwn(mapping, key) ? mapping[key] : key, input[key]];
    }
  }
}

/** Encodes declared own keys; null/undefined scalar values are omitted. */
export function encodeQuery(tool, input) {
  const query = new URLSearchParams();
  for (const [key, value] of parameters(tool, input)) {
    if (value === null || value === undefined) continue;
    if (Array.isArray(value)) {
      const name = tool.endpoint.arrayFormat === 'brackets' ? `${key}[]` : key;
      for (const item of value) query.append(name, String(item));
    } else {
      query.append(key, String(value));
    }
  }
  return query.toString();
}

function csrfToken(csrf, doc) {
  if (csrf.source === 'meta') {
    const name = csrf.name.replace(/["\\]/g, '\\$&');
    return doc?.querySelector(`meta[name="${name}"]`)?.content;
  }
  if (csrf.source === 'cookie') {
    for (const part of (doc?.cookie ?? '').split(';')) {
      const cookie = part.trim();
      const separator = cookie.indexOf('=');
      if (separator < 0 || cookie.slice(0, separator) !== csrf.name) continue;
      const value = cookie.slice(separator + 1);
      try {
        return decodeURIComponent(value);
      } catch {
        return value;
      }
    }
  }
  return undefined;
}

/** Returns { url, options } or a pre-dispatch failure. The caller supplies signal. */
export function buildRequest(tool, input, transport, doc) {
  const validation = validateInput(tool, input);
  if (!validation.ok) return validation;
  const endpoint = tool.endpoint;
  const path = endpoint?.path;
  if (typeof path !== 'string' || !path.startsWith('/') || path.startsWith('//') ||
      /[\\\u0000-\u001f\u007f]/.test(path)) {
    return failure('invalid_input');
  }
  let url;
  try {
    const origin = doc?.location?.origin ?? globalThis.location?.origin;
    url = new URL(path, origin);
    if (url.origin !== origin) return failure('invalid_input');
  } catch {
    return failure('invalid_input');
  }
  const method = endpoint.method.toUpperCase();
  const headers = { Accept: 'application/json' };
  const options = { method, mode: 'same-origin', credentials: 'same-origin', redirect: 'error', headers };
  const csrf = transport?.csrf;
  if (csrf) {
    const token = csrfToken(csrf, doc);
    if (!token && method !== 'GET') return failure('csrf_token_missing');
    if (token) headers[csrf.header] = token;
  }
  if (method === 'GET') {
    for (const [key, value] of new URLSearchParams(encodeQuery(tool, input))) {
      url.searchParams.append(key, value);
    }
  } else {
    headers['Content-Type'] = 'application/json';
    options.body = JSON.stringify(Object.fromEntries(parameters(tool, input)));
  }
  return { url: url.href, options };
}

/**
 * Pure outcome conversion. phase is 'before-dispatch', 'after-dispatch' (errors),
 * or 'response' (default). A parsed response is { status, data, invalidResponse? }.
 */
export function toOutcome(tool, value, phase = 'response') {
  const readOnly = tool.annotations?.readOnlyHint === true;
  if (phase === 'before-dispatch') {
    const code = value?.name === 'AbortError' ? 'aborted'
      : value?.code === 'csrf_token_missing' ? 'csrf_token_missing' : 'invalid_input';
    return failure(code);
  }
  if (phase === 'after-dispatch') {
    return failure(readOnly
      ? value?.name === 'AbortError' ? 'aborted' : 'network_error'
      : 'unknown_outcome');
  }
  const { status, data, invalidResponse } = value;
  if (status < 200 || status >= 300) return failure('http_error', status);
  if (status === 204) return { ok: true, status, data: null };
  let omitted = invalidResponse ? 'invalid_response' : undefined;
  if (!omitted && tool.maxResponseChars !== undefined) {
    try {
      if (JSON.stringify(data).length > tool.maxResponseChars) omitted = 'response_too_large';
    } catch {
      omitted = 'invalid_response';
    }
  }
  if (!omitted) return { ok: true, status, data };
  if (readOnly) return failure(omitted, status);
  return {
    ok: true, status, data: null, dataOmitted: omitted,
    message: 'The operation succeeded. Do not repeat it.',
  };
}

function executor(tool, transport, doc, fetcher) {
  return async (inputObject, options = {}) => {
    let phase = 'before-dispatch';
    try {
      const request = buildRequest(tool, inputObject, transport, doc);
      if (request.ok === false) return request;
      const signal = options?.signal;
      if (signal?.aborted) return toOutcome(tool, { name: 'AbortError' }, phase);
      phase = 'after-dispatch';
      const response = await fetcher(request.url, { ...request.options, signal });
      const status = response.status;
      if (status === 204 || status < 200 || status >= 300) {
        return toOutcome(tool, { status });
      }
      let data;
      try {
        data = await response.json();
      } catch (error) {
        // A body read can itself fail after dispatch; only a JSON parse failure
        // proves that the received response was non-JSON.
        if (error?.name !== 'SyntaxError') throw error;
        return toOutcome(tool, { status, invalidResponse: true });
      }
      return toOutcome(tool, { status, data });
    } catch (error) {
      return toOutcome(tool, error, phase);
    }
  };
}

export function mount({
  selector = '#webmcp-manifest', modelContext, fetch: fetcher = globalThis.fetch,
  document: doc = globalThis.document,
} = {}) {
  const context = modelContext === undefined
    ? doc?.modelContext ?? globalThis.navigator?.modelContext : modelContext;
  if (!context) return { refresh: async () => {}, dispose() {} };

  const registrations = new Map();
  let queue = Promise.resolve();
  let generation = 0;
  let disposed = false;
  let warnedVersion = false;
  let finishDisposal;
  const disposal = new Promise(resolve => { finishDisposal = resolve; });

  function remove(name, entry) {
    entry.controller.abort();
    if (registrations.get(name) === entry) registrations.delete(name);
  }

  function readManifest() {
    try {
      const element = doc?.querySelector(selector);
      if (!element) return { tools: [] };
      const manifest = JSON.parse(element.textContent);
      if (manifest?.webmcpManifestVersion !== 1) {
        if (!warnedVersion) {
          warnedVersion = true;
          console.warn('WebMCP: unsupported manifest version; no tools registered.');
        }
        return { tools: [] };
      }
      if (!Array.isArray(manifest.tools)) throw new TypeError('Manifest tools must be an array.');
      return manifest;
    } catch (error) {
      console.warn('WebMCP: could not read the manifest.', error);
      return { tools: [] };
    }
  }

  async function reconcile(currentGeneration) {
    if (disposed || currentGeneration !== generation) return;
    const manifest = readManifest();
    const desired = new Map(manifest.tools.map(tool => [tool.name, tool]));
    for (const [name, entry] of registrations) {
      if (!desired.has(name) || desired.get(name).fingerprint !== entry.fingerprint) {
        remove(name, entry);
      }
    }
    for (const [name, tool] of desired) {
      if (disposed || currentGeneration !== generation) return;
      if (registrations.has(name)) continue;
      const entry = { fingerprint: tool.fingerprint, controller: new AbortController() };
      registrations.set(name, entry);
      const registration = (async () => {
        try {
          const definition = {
            name: tool.name, description: tool.description, inputSchema: tool.inputSchema,
            execute: executor(tool, manifest.transport, doc, fetcher),
          };
          if (tool.title !== undefined) definition.title = tool.title;
          if (tool.annotations !== undefined) definition.annotations = tool.annotations;
          await context.registerTool(definition, { signal: entry.controller.signal });
          if (disposed || currentGeneration !== generation) remove(name, entry);
        } catch (error) {
          remove(name, entry);
          console.warn(`WebMCP: could not register tool "${name}".`, error);
        }
      })();
      // Disposal releases callers even if an implementation never settles its
      // registerTool promise. The handler above still cleans up a late result.
      await Promise.race([registration, disposal]);
    }
  }

  function refresh() {
    if (disposed) return Promise.resolve();
    const currentGeneration = ++generation;
    queue = queue.then(() => reconcile(currentGeneration)).catch(error => {
      console.warn('WebMCP: could not refresh tools.', error);
    });
    return queue;
  }

  function dispose() {
    if (disposed) return;
    disposed = true;
    ++generation;
    doc?.removeEventListener('turbo:load', onTurboLoad);
    for (const [name, entry] of registrations) remove(name, entry);
    queue = Promise.resolve();
    finishDisposal();
  }

  function onTurboLoad() { void refresh(); }
  doc?.addEventListener('turbo:load', onTurboLoad);
  void refresh();
  return { refresh, dispose };
}

// Opt in through markup so strict CSP applications need no inline bootstrap.
if (globalThis.document) {
  const doc = globalThis.document;
  const started = Symbol.for('webmcp.runtime.autostart');
  const autostart = () => {
    if (globalThis[started]) return;
    // Turbo keeps this module loaded across visits, so a page without an
    // opted-in manifest (a sign-in page, say) waits for a later visit that has one.
    if (!doc.querySelector('#webmcp-manifest')?.hasAttribute('data-webmcp-autostart')) {
      doc.addEventListener('turbo:load', autostart);
      return;
    }
    doc.removeEventListener('turbo:load', autostart);
    globalThis[started] = true;
    const handle = mount();
    globalThis.WebMCPRuntime = { handle, mount };
    doc.dispatchEvent(new CustomEvent('webmcp:mounted', { detail: handle }));
  };
  if (doc.readyState === 'loading') {
    doc.addEventListener('DOMContentLoaded', autostart, { once: true });
  } else {
    autostart();
  }
}
