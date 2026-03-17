import React, { useCallback, useEffect, useRef, useState } from 'react';
import { useQuery } from '@tanstack/react-query';
import { configApi } from '../api/client';
import { useToast } from '../components/useToast';

// ---------------------------------------------------------------------------
// Token definitions
// ---------------------------------------------------------------------------

interface TokenDef {
  token: string;
  description: string;
  example: string;
}

const TOKENS: TokenDef[] = [
  { token: '{title}',           description: 'Scene title (raw)',                      example: 'Example Scene Title' },
  { token: '{title_slug}',      description: 'URL-safe scene title',                   example: 'example-scene-title' },
  { token: '{studio}',          description: 'Studio name (raw)',                       example: 'Example Studio' },
  { token: '{studio_slug}',     description: 'URL-safe studio name',                   example: 'example-studio' },
  { token: '{performer}',       description: 'First performer (surname-sorted)',         example: 'Jane Doe' },
  { token: '{performers}',      description: 'All performers, comma-separated (max 3)', example: 'Jane Doe, John Smith' },
  { token: '{performers_slug}', description: 'Slug version of performers',              example: 'jane-doe-john-smith' },
  { token: '{date}',            description: 'Full release date',                       example: '2024-03-15' },
  { token: '{year}',            description: '4-digit year from release date',          example: '2024' },
  { token: '{month}',           description: '2-digit month from release date',         example: '03' },
  { token: '{resolution}',      description: 'Detected video resolution',               example: '1080p' },
  { token: '{ext}',             description: 'File extension (no leading dot)',         example: 'mp4' },
];

const KNOWN_TOKENS = new Set(TOKENS.map((t) => t.token));

// ---------------------------------------------------------------------------
// Synthetic preview data
// ---------------------------------------------------------------------------

interface PreviewData {
  title: string;
  title_slug: string;
  studio: string;
  studio_slug: string;
  performer: string;
  performers: string;
  performers_slug: string;
  date: string;
  year: string;
  month: string;
  day: string;
  resolution: string;
  ext: string;
  duration: string;
}

const PREVIEW_DATA: PreviewData = {
  title:           'Example Scene Title',
  title_slug:      'example-scene-title',
  studio:          'Example Studio',
  studio_slug:     'example-studio',
  performer:       'Jane Doe',
  performers:      'Jane Doe, John Smith',
  performers_slug: 'jane-doe-john-smith',
  date:            '2024-03-15',
  year:            '2024',
  month:           '03',
  day:             '15',
  resolution:      '1080p',
  ext:             'mp4',
  duration:        '01:02:00',
};

// ---------------------------------------------------------------------------
// Template substitution
// ---------------------------------------------------------------------------

interface PreviewSegment {
  text: string;
  unknown: boolean;
}

function parseTemplate(template: string): PreviewSegment[] {
  const segments: PreviewSegment[] = [];
  const regex = /\{([^}]+)\}/g;
  let lastIndex = 0;
  let match: RegExpExecArray | null;

  while ((match = regex.exec(template)) !== null) {
    if (match.index > lastIndex) {
      segments.push({ text: template.slice(lastIndex, match.index), unknown: false });
    }

    const fullToken = match[0];
    const key = match[1];
    const dataKey = key as keyof PreviewData;

    if (KNOWN_TOKENS.has(fullToken) && dataKey in PREVIEW_DATA) {
      segments.push({ text: PREVIEW_DATA[dataKey], unknown: false });
    } else {
      segments.push({ text: fullToken, unknown: true });
    }

    lastIndex = regex.lastIndex;
  }

  if (lastIndex < template.length) {
    segments.push({ text: template.slice(lastIndex), unknown: false });
  }

  return segments;
}

// ---------------------------------------------------------------------------
// Validation
// ---------------------------------------------------------------------------

interface ValidationResult {
  errors: string[];
  warnings: string[];
}

function validateTemplate(template: string): ValidationResult {
  const errors: string[] = [];
  const warnings: string[] = [];

  if (!template.trim()) {
    errors.push('Template cannot be empty.');
    return { errors, warnings };
  }

  if (template.startsWith('/')) {
    errors.push('Template must not start with /.');
  }
  if (template.endsWith('/')) {
    errors.push('Template must not end with /.');
  }

  const tokenMatches = [...template.matchAll(/\{([^}]+)\}/g)];
  const unknownTokens: string[] = [];
  for (const m of tokenMatches) {
    const full = m[0];
    if (!KNOWN_TOKENS.has(full)) {
      unknownTokens.push(full);
    }
  }
  if (unknownTokens.length > 0) {
    warnings.push(`Unknown token(s): ${unknownTokens.join(', ')}`);
  }

  const segments = template.split('/');
  const lastSegment = segments[segments.length - 1];
  if (!lastSegment.includes('{ext}')) {
    errors.push('The filename (last segment) must contain {ext}.');
  }

  const stripped = lastSegment.replace(/\{[^}]+\}/g, '').trim();
  if (stripped.length === 0) {
    errors.push('The filename (last segment) must contain at least one non-token character.');
  }

  return { errors, warnings };
}

// ---------------------------------------------------------------------------
// Preview renderer (React nodes)
// ---------------------------------------------------------------------------

function renderPreview(template: string): React.ReactNode {
  if (!template.trim()) {
    return <span className="text-gray-400 dark:text-zinc-500 italic">Enter a template to see preview</span>;
  }

  const segments = parseTemplate(template);
  return segments.map((seg, i) =>
    seg.unknown ? (
      <span key={i} className="text-red-500 dark:text-red-400 font-semibold">{seg.text}</span>
    ) : (
      <span key={i}>{seg.text}</span>
    ),
  );
}

function getRenderedString(template: string): string {
  if (!template.trim()) return '';
  return parseTemplate(template)
    .map((s) => s.text)
    .join('');
}

// ---------------------------------------------------------------------------
// Character count helper
// ---------------------------------------------------------------------------

function getFilenameSegment(rendered: string): string {
  const idx = rendered.lastIndexOf('/');
  return idx === -1 ? rendered : rendered.slice(idx + 1);
}

// ---------------------------------------------------------------------------
// Component
// ---------------------------------------------------------------------------

export default function TemplateBuilder() {
  const { toast } = useToast();

  const { data: configData, isLoading: configLoading } = useQuery({
    queryKey: ['config'],
    queryFn: () => configApi.get(),
  });

  const [template, setTemplate] = useState<string>('');
  const [saving, setSaving] = useState(false);
  const textareaRef = useRef<HTMLTextAreaElement>(null);

  useEffect(() => {
    if (configData) {
      const loaded = configData?.directory?.template ?? '';
      setTemplate(loaded);
    }
  }, [configData]);

  const validation = validateTemplate(template);
  const renderedString = getRenderedString(template);
  const filenameSegment = getFilenameSegment(renderedString);
  const charCount = filenameSegment.length;
  const charWarning = charCount > 200;

  const insertToken = useCallback((token: string) => {
    const ta = textareaRef.current;
    if (!ta) {
      setTemplate((prev) => prev + token);
      return;
    }
    const start = ta.selectionStart ?? template.length;
    const end = ta.selectionEnd ?? template.length;
    const next = template.slice(0, start) + token + template.slice(end);
    setTemplate(next);
    requestAnimationFrame(() => {
      ta.focus();
      const pos = start + token.length;
      ta.setSelectionRange(pos, pos);
    });
  }, [template]);

  const handleSave = async () => {
    if (validation.errors.length > 0) {
      toast('Fix validation errors before saving.', 'error');
      return;
    }
    setSaving(true);
    try {
      await configApi.update({ 'directory.template': template } as Record<string, string>);
      toast('Template saved.', 'success');
    } catch (err) {
      const msg = err instanceof Error ? err.message : 'Unknown error';
      toast(`Save failed: ${msg}`, 'error');
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="p-6 max-w-7xl mx-auto">
      <h1 className="text-xl font-semibold text-gray-900 dark:text-white mb-6">Directory Template Builder</h1>

      {configLoading && (
        <p className="text-gray-500 dark:text-zinc-400 mb-4">Loading current template…</p>
      )}

      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        {/* ── Left column ── */}
        <div className="flex flex-col gap-6">
          {/* Template input */}
          <div className="bg-gray-50 dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700 rounded-lg p-4 flex flex-col gap-3">
            <label htmlFor="template-input" className="text-sm font-medium text-gray-700 dark:text-zinc-300">
              Template
            </label>
            <textarea
              id="template-input"
              ref={textareaRef}
              value={template}
              onChange={(e) => setTemplate(e.target.value)}
              rows={4}
              spellCheck={false}
              placeholder="{studio}/{year}/{title} ({year}).{ext}"
              className="w-full bg-white dark:bg-zinc-800 border border-gray-300 dark:border-zinc-600 rounded px-3 py-2 text-sm text-gray-900 dark:text-white font-mono placeholder-gray-400 dark:placeholder-zinc-500 focus:outline-none focus:ring-2 focus:ring-indigo-500 resize-y"
            />

            {/* Validation errors */}
            {validation.errors.length > 0 && (
              <ul className="flex flex-col gap-1">
                {validation.errors.map((e, i) => (
                  <li key={i} className="text-sm text-red-600 dark:text-red-400 flex items-start gap-1">
                    <span className="mt-0.5 shrink-0">✕</span>
                    <span>{e}</span>
                  </li>
                ))}
              </ul>
            )}

            {/* Validation warnings */}
            {validation.warnings.length > 0 && (
              <ul className="flex flex-col gap-1">
                {validation.warnings.map((w, i) => (
                  <li key={i} className="text-sm text-amber-600 dark:text-amber-400 flex items-start gap-1">
                    <span className="mt-0.5 shrink-0">⚠</span>
                    <span>{w}</span>
                  </li>
                ))}
              </ul>
            )}

            <button
              onClick={handleSave}
              disabled={saving || validation.errors.length > 0}
              className="self-start px-4 py-2 rounded bg-indigo-600 hover:bg-indigo-500 disabled:opacity-50 disabled:cursor-not-allowed text-white text-sm font-medium transition-colors"
            >
              {saving ? 'Saving…' : 'Save Template'}
            </button>
          </div>

          {/* Token reference table */}
          <div className="bg-gray-50 dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700 rounded-lg p-4 flex flex-col gap-3">
            <p className="text-sm font-medium text-gray-700 dark:text-zinc-300">
              Token Reference
              <span className="ml-2 text-xs text-gray-400 dark:text-zinc-500 font-normal">Click a token to insert it at cursor</span>
            </p>
            <div className="overflow-x-auto">
              <table className="w-full text-sm">
                <thead>
                  <tr className="border-b border-gray-200 dark:border-zinc-700">
                    <th className="text-left py-2 pr-4 text-gray-500 dark:text-zinc-400 font-medium whitespace-nowrap">Token</th>
                    <th className="text-left py-2 pr-4 text-gray-500 dark:text-zinc-400 font-medium">Description</th>
                    <th className="text-left py-2 text-gray-500 dark:text-zinc-400 font-medium whitespace-nowrap">Example</th>
                  </tr>
                </thead>
                <tbody>
                  {TOKENS.map((t) => (
                    <tr key={t.token} className="border-b border-gray-100 dark:border-zinc-800 hover:bg-gray-100 dark:hover:bg-zinc-800/50 transition-colors">
                      <td className="py-2 pr-4 whitespace-nowrap">
                        <button
                          onClick={() => insertToken(t.token)}
                          className="font-mono text-indigo-600 dark:text-indigo-400 hover:text-indigo-500 dark:hover:text-indigo-300 hover:underline cursor-pointer focus:outline-none focus:ring-1 focus:ring-indigo-500 rounded px-0.5"
                          title={`Insert ${t.token}`}
                        >
                          {t.token}
                        </button>
                      </td>
                      <td className="py-2 pr-4 text-gray-600 dark:text-zinc-300">{t.description}</td>
                      <td className="py-2 text-gray-500 dark:text-zinc-400 font-mono text-xs whitespace-nowrap">{t.example}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          </div>
        </div>

        {/* ── Right column: Live preview ── */}
        <div className="flex flex-col gap-4">
          <div className="bg-gray-50 dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700 rounded-lg p-4 flex flex-col gap-4">
            <p className="text-sm font-medium text-gray-700 dark:text-zinc-300">Live Preview</p>

            {/* Synthetic data reference */}
            <div className="bg-gray-100 dark:bg-zinc-800 rounded p-3 text-xs text-gray-500 dark:text-zinc-400 flex flex-col gap-1">
              <p className="text-gray-400 dark:text-zinc-500 font-medium mb-1 uppercase tracking-wide text-[11px]">Synthetic scene data</p>
              <div className="grid grid-cols-2 gap-x-4 gap-y-0.5">
                <span><span className="text-gray-400 dark:text-zinc-500">Studio:</span> {PREVIEW_DATA.studio}</span>
                <span><span className="text-gray-400 dark:text-zinc-500">Title:</span> {PREVIEW_DATA.title}</span>
                <span><span className="text-gray-400 dark:text-zinc-500">Performers:</span> {PREVIEW_DATA.performers}</span>
                <span><span className="text-gray-400 dark:text-zinc-500">Date:</span> {PREVIEW_DATA.date}</span>
                <span><span className="text-gray-400 dark:text-zinc-500">Resolution:</span> {PREVIEW_DATA.resolution}</span>
                <span><span className="text-gray-400 dark:text-zinc-500">Extension:</span> {PREVIEW_DATA.ext}</span>
                <span><span className="text-gray-400 dark:text-zinc-500">Duration:</span> {PREVIEW_DATA.duration}</span>
              </div>
            </div>

            {/* Rendered path */}
            <div>
              <p className="text-xs text-gray-400 dark:text-zinc-500 mb-1 uppercase tracking-wide font-medium">Rendered path</p>
              <div className="bg-gray-100 dark:bg-zinc-800 rounded px-3 py-2 font-mono text-sm text-gray-900 dark:text-zinc-100 break-all leading-relaxed min-h-[2.5rem]">
                {renderPreview(template)}
              </div>
            </div>

            {/* Filename segment + character count */}
            {renderedString && (
              <div>
                <p className="text-xs text-gray-400 dark:text-zinc-500 mb-1 uppercase tracking-wide font-medium">Filename segment</p>
                <div className="bg-gray-100 dark:bg-zinc-800 rounded px-3 py-2 font-mono text-sm text-gray-700 dark:text-zinc-300 break-all">
                  {filenameSegment || <span className="text-gray-400 dark:text-zinc-500 italic">—</span>}
                </div>
                <div className={`mt-1.5 text-xs flex items-center gap-1 ${charWarning ? 'text-amber-500 dark:text-amber-400' : 'text-gray-400 dark:text-zinc-500'}`}>
                  {charWarning && <span>⚠</span>}
                  <span>{charCount} character{charCount !== 1 ? 's' : ''}</span>
                  {charWarning && <span>(warning: exceeds 200 character limit)</span>}
                </div>
              </div>
            )}

            {/* Legend */}
            <div className="flex items-center gap-4 text-xs text-gray-400 dark:text-zinc-500 pt-1 border-t border-gray-200 dark:border-zinc-800">
              <span className="flex items-center gap-1">
                <span className="text-gray-700 dark:text-zinc-200 font-mono">text</span>
                <span>= substituted value</span>
              </span>
              <span className="flex items-center gap-1">
                <span className="text-red-500 dark:text-red-400 font-mono font-semibold">{'{unknown}'}</span>
                <span>= unknown token</span>
              </span>
            </div>
          </div>

          {/* Example templates */}
          <div className="bg-gray-50 dark:bg-zinc-900 border border-gray-200 dark:border-zinc-700 rounded-lg p-4 flex flex-col gap-3">
            <p className="text-sm font-medium text-gray-700 dark:text-zinc-300">Example Templates</p>
            <ul className="flex flex-col gap-2">
              {[
                { label: 'Studio › Year › Title',      value: '{studio}/{year}/{title} ({year}).{ext}' },
                { label: 'Performer-first',             value: '{performer}/{studio}/{title} ({date}).{ext}' },
                { label: 'Flat with rich filename',    value: '{studio} - {title} - {performers} ({year}) [{resolution}].{ext}' },
                { label: 'Date-based organisation',    value: '{year}/{month}/{studio}/{title}.{ext}' },
              ].map((ex) => (
                <li key={ex.value}>
                  <button
                    onClick={() => setTemplate(ex.value)}
                    className="text-left w-full group"
                  >
                    <span className="block text-xs text-gray-500 dark:text-zinc-400 group-hover:text-gray-700 dark:group-hover:text-zinc-200 transition-colors">{ex.label}</span>
                    <span className="block font-mono text-xs text-indigo-600 dark:text-indigo-400 group-hover:text-indigo-500 dark:group-hover:text-indigo-300 transition-colors">{ex.value}</span>
                  </button>
                </li>
              ))}
            </ul>
          </div>
        </div>
      </div>
    </div>
  );
}
