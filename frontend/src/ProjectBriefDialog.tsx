import {useEffect, useRef, useState} from "react";
import {GenerateProjectBrief, ProjectDocs} from "../wailsjs/go/main/App";
import {brief} from "../wailsjs/go/models";

// MAX_FILES and MAX_PROMPT_TOKENS mirror maxFiles and maxPromptTokens in
// internal/brief/brief.go; the backend enforces them too.
const MAX_FILES = 5;
const MAX_PROMPT_TOKENS = 100_000;

type DocsLoad =
    | { kind: "loading" }
    | { kind: "error"; message: string }
    | { kind: "loaded"; docs: brief.DocFile[] };

function suggestedPaths(docs: brief.DocFile[]): Set<string> {
    return new Set(docs.filter((d) => d.suggested).slice(0, MAX_FILES).map((d) => d.path));
}

// The stored brief's files that still exist, or the suggested files when none remain.
function initialPaths(docs: brief.DocFile[], state: brief.State | null): Set<string> {
    const stored = state?.Brief.files ?? [];
    const previous = docs.filter((d) => stored.includes(d.path)).map((d) => d.path);
    return previous.length > 0 ? new Set(previous) : suggestedPaths(docs);
}

// Doc paths are slash-separated; dir keeps its trailing slash and is empty at the repo root.
function splitPath(path: string): { dir: string; name: string } {
    const slash = path.lastIndexOf("/");
    return {dir: path.slice(0, slash + 1), name: path.slice(slash + 1)};
}

interface ProjectBriefDialogProps {
    state: brief.State | null;
    onGenerated: (state: brief.State) => void;
    onClose: () => void;
}

function ProjectBriefDialog(props: ProjectBriefDialogProps) {
    const {state, onGenerated, onClose} = props;
    const [load, setLoad] = useState<DocsLoad>({kind: "loading"});
    const [selected, setSelected] = useState<Set<string>>(new Set());
    const [generating, setGenerating] = useState(false);
    const [error, setError] = useState("");
    // A generation that outlives this dialog must not touch the parent state.
    const mounted = useRef(false);

    useEffect(() => {
        mounted.current = true;
        return () => {
            mounted.current = false;
        };
    }, []);

    // The parent mounts this dialog on every open, so the selection starts
    // from the brief state of that moment.
    useEffect(() => {
        ProjectDocs()
            .then((docs) => {
                setLoad({kind: "loaded", docs});
                setSelected(initialPaths(docs, state));
            })
            .catch((err) => setLoad({kind: "error", message: String(err)}));
    }, []);

    useEffect(() => {
        if (generating) {
            return;
        }
        function onKeyDown(e: KeyboardEvent) {
            if (e.key === "Escape") {
                onClose();
            }
        }
        window.addEventListener("keydown", onKeyDown);
        return () => window.removeEventListener("keydown", onKeyDown);
    }, [generating, onClose]);

    const docs = load.kind === "loaded" ? load.docs : [];
    const files = docs.filter((d) => selected.has(d.path));
    const tokens = Math.round(files.reduce((sum, d) => sum + d.size, 0) / 4);
    const overBudget = tokens > MAX_PROMPT_TOKENS;
    const atLimit = files.length >= MAX_FILES;

    function toggle(path: string) {
        const next = new Set(selected);
        if (!next.delete(path)) {
            next.add(path);
        }
        setSelected(next);
    }

    function generate() {
        setGenerating(true);
        setError("");
        GenerateProjectBrief(files.map((d) => d.path))
            .then((next) => {
                if (mounted.current) {
                    onGenerated(next);
                }
            })
            .catch((err) => setError(String(err)))
            .finally(() => setGenerating(false));
    }

    return (
        <div className="config-overlay" onClick={generating ? undefined : onClose}>
            <div className="config-dialog project-brief-dialog" onClick={(e) => e.stopPropagation()}>
                <h2>Project brief</h2>
                <p className="config-subtitle">
                    Select the documentation files that describe this project. The LLM writes the brief from them.
                </p>
                {load.kind === "loading" && (
                    <p className="project-brief-dialog-hint">Loading documentation files…</p>
                )}
                {load.kind === "error" && (
                    <p className="explain-error" role="alert">{load.message}</p>
                )}
                {load.kind === "loaded" && docs.length === 0 && (
                    <p className="project-brief-dialog-hint">No documentation files found in this repository.</p>
                )}
                {docs.length > 0 && (
                    <>
                        <div className="project-brief-dialog-selection">
                            <button type="button" disabled={generating} onClick={() => setSelected(suggestedPaths(docs))}>
                                Select suggested
                            </button>
                            <button type="button" disabled={generating} onClick={() => setSelected(new Set())}>
                                None
                            </button>
                        </div>
                        <ul className="project-brief-doc-list">
                            {docs.map((d) => {
                                const {dir, name} = splitPath(d.path);
                                return (
                                    <li key={d.path}>
                                        <label className="project-brief-doc">
                                            <input
                                                type="checkbox"
                                                checked={selected.has(d.path)}
                                                disabled={generating || (atLimit && !selected.has(d.path))}
                                                onChange={() => toggle(d.path)}
                                            />
                                            <span className="project-brief-doc-path" title={d.path}>
                                                <span className="project-brief-doc-dir">{dir}</span>
                                                {name}
                                            </span>
                                            <span className="project-brief-doc-size">{Math.ceil(d.size / 1024)} KB</span>
                                        </label>
                                    </li>
                                );
                            })}
                        </ul>
                        <p className="project-brief-dialog-hint">
                            {files.length}/{MAX_FILES} files · ~{tokens} tokens
                            {atLimit && ` · ${MAX_FILES} files maximum`}
                            {overBudget && (
                                <span className="project-brief-dialog-over"> · Too many tokens. Select fewer files.</span>
                            )}
                        </p>
                        <p className="project-brief-dialog-hint">The text of the selected files is sent to the LLM.</p>
                    </>
                )}
                <div className="config-dialog-actions">
                    <button type="button" disabled={generating} onClick={onClose}>Cancel</button>
                    <button type="button" disabled={files.length === 0 || overBudget || generating} onClick={generate}>
                        {generating ? "Generating…" : state?.Stored ? "Regenerate" : "Generate"}
                    </button>
                </div>
                {error && <p className="explain-error" role="alert">{error}</p>}
            </div>
        </div>
    );
}

export default ProjectBriefDialog;
