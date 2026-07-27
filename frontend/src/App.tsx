import {useEffect, useState} from 'react';
import './App.css';
import {
    APIKeyUsedFallback,
    ChangedFiles,
    ExplainFile,
    FileDiff,
    GetConfig,
    HasAPIKey,
    SaveConfig,
    SetAPIKey,
} from "../wailsjs/go/main/App";
import {config, gitdiff} from "../wailsjs/go/models";

const typeLabels: Record<string, string> = {
    Modified: "M",
    Added: "A",
    Deleted: "D",
    Renamed: "R",
};

function App() {
    const [files, setFiles] = useState<gitdiff.FileChange[]>([]);
    const [selectedPath, setSelectedPath] = useState<string | null>(null);
    const [diffText, setDiffText] = useState<string>("");

    const [cfg, setCfg] = useState<config.Config>(new config.Config());
    const [hasKey, setHasKey] = useState(false);
    const [usedFallback, setUsedFallback] = useState(false);
    const [configDialogOpen, setConfigDialogOpen] = useState(false);

    const [explanation, setExplanation] = useState<string>("");
    const [explaining, setExplaining] = useState(false);
    const [explainError, setExplainError] = useState<string>("");

    const [bannerDismissed, setBannerDismissed] = useState(false);

    const ready = cfg.base_url !== "" && cfg.model !== "" && hasKey;

    useEffect(() => {
        ChangedFiles().then((f) => setFiles(f ?? []));
        checkReadiness();
    }, []);

    function checkReadiness() {
        GetConfig().then(setCfg);
        HasAPIKey().then(setHasKey);
        APIKeyUsedFallback().then(setUsedFallback);
    }

    function selectFile(path: string) {
        setSelectedPath(path);
        setDiffText("");
        FileDiff(path).then(setDiffText);
        setExplanation("");
        setExplainError("");
    }

    function explain() {
        if (!selectedPath) {
            return;
        }
        setExplaining(true);
        setExplanation("");
        setExplainError("");
        ExplainFile(selectedPath)
            .then(setExplanation)
            .catch((err) => setExplainError(String(err)))
            .finally(() => setExplaining(false));
    }

    function openConfigDialog() {
        setConfigDialogOpen(true);
    }

    function closeConfigDialog() {
        setConfigDialogOpen(false);
        checkReadiness();
    }

    return (
        <div id="App-root">
            {!ready && !bannerDismissed && (
                <div className="readiness-banner">
                    <span className="readiness-banner-icon">⚠</span>
                    <span className="readiness-banner-text">
                        AI features are disabled.
                        {!hasKey && usedFallback && " (API key loaded from environment variable fallback — OS keyring unavailable.)"}
                    </span>
                    <button className="readiness-banner-action" onClick={openConfigDialog}>Config</button>
                    <button className="readiness-banner-action" onClick={() => setBannerDismissed(true)}>Dismiss</button>
                </div>
            )}
            <div id="App">
                <div className="rail">
                    <ul className="file-list">
                        {files.map((file) => (
                            <li
                                key={file.Path}
                                className={file.Path === selectedPath ? "file-item selected" : "file-item"}
                                onClick={() => selectFile(file.Path)}
                            >
                                <span className="file-type">{typeLabels[file.Type] ?? file.Type}</span>
                                <span className="file-path">{file.Path}</span>
                            </li>
                        ))}
                    </ul>
                </div>
                <div className="diff-pane">
                    <pre>{diffText}</pre>
                </div>
                <div className="explanation-pane">
                    <div className="explanation-header">
                        <button
                            className="explain-button"
                            disabled={!ready || !selectedPath || explaining}
                            onClick={explain}
                        >
                            {explaining ? "Explaining…" : "Explain"}
                        </button>
                    </div>
                    {!selectedPath && (
                        <p className="placeholder">Select a file to see its explanation</p>
                    )}
                    {selectedPath && !explaining && !explanation && !explainError && (
                        <p className="placeholder">Click Explain to see an explanation of this diff</p>
                    )}
                    {explaining && (
                        <p className="placeholder">Explaining…</p>
                    )}
                    {explainError && (
                        <p className="explain-error">{explainError}</p>
                    )}
                    {explanation && explanation.split(/\n\n+/).map((paragraph, i) => (
                        <p key={i}>{paragraph}</p>
                    ))}
                </div>
            </div>
            {configDialogOpen && (
                <ConfigDialog
                    initialConfig={cfg}
                    hasKey={hasKey}
                    usedFallback={usedFallback}
                    onClose={closeConfigDialog}
                />
            )}
        </div>
    )
}

function ConfigDialog(props: {
    initialConfig: config.Config;
    hasKey: boolean;
    usedFallback: boolean;
    onClose: () => void;
}) {
    const [baseURL, setBaseURL] = useState(props.initialConfig.base_url);
    const [model, setModel] = useState(props.initialConfig.model);
    const [newKey, setNewKey] = useState("");

    function save() {
        const cfg = new config.Config({base_url: baseURL, model});
        const tasks: Promise<unknown>[] = [SaveConfig(cfg)];
        if (newKey !== "") {
            tasks.push(SetAPIKey(newKey));
        }
        Promise.all(tasks).then(props.onClose);
    }

    return (
        <div className="config-overlay" onClick={props.onClose}>
            <div className="config-dialog" onClick={(e) => e.stopPropagation()}>
                <h2>Explain settings</h2>
                <label className="config-field">
                    Base URL
                    <input
                        type="text"
                        value={baseURL}
                        onChange={(e) => setBaseURL(e.target.value)}
                    />
                </label>
                <label className="config-field">
                    Model
                    <input
                        type="text"
                        value={model}
                        onChange={(e) => setModel(e.target.value)}
                    />
                </label>
                <label className="config-field">
                    {props.hasKey
                        ? props.usedFallback
                            ? "API key: set (via environment fallback)"
                            : "API key: set ✓"
                        : "API key: not set"}
                    <input
                        type="password"
                        placeholder={props.hasKey ? "Enter a new key to replace it" : "Enter API key"}
                        value={newKey}
                        onChange={(e) => setNewKey(e.target.value)}
                    />
                </label>
                <div className="config-dialog-actions">
                    <button onClick={props.onClose}>Cancel</button>
                    <button onClick={save}>Save</button>
                </div>
            </div>
        </div>
    )
}

export default App
