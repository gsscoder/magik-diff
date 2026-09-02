import {gitdiff} from "../wailsjs/go/models";

interface FileStatus {
    glyph: string;
    label: string;
    className: string;
}

// Keys match the lowercase ChangeType values serialized by the Go backend.
const statusStyles: Record<string, FileStatus> = {
    modified: {glyph: "●", label: "Modified", className: "status-modified"},
    added: {glyph: "+", label: "Added", className: "status-added"},
    deleted: {glyph: "−", label: "Deleted", className: "status-deleted"},
    renamed: {glyph: "→", label: "Renamed", className: "status-renamed"},
};

const fallbackStatus: FileStatus = {glyph: "?", label: "Changed", className: ""};

function DocIcon() {
    return (
        <svg width="12" height="12" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <path d="M4 1.5h5l3 3v10a1 1 0 0 1-1 1H4a1 1 0 0 1-1-1v-12a1 1 0 0 1 1-1Z" stroke="currentColor" />
            <path d="M9 1.5V4a1 1 0 0 0 1 1h2.5" stroke="currentColor" />
        </svg>
    );
}

function matchesFilter(path: string, filter: string): boolean {
    if (!/[*?]/.test(filter)) {
        return path.toLowerCase().includes(filter.toLowerCase());
    }
    const pattern = filter.replace(/[.+^${}()|[\]\\]/g, "\\$&").replace(/\*/g, ".*").replace(/\?/g, ".");
    return new RegExp(`^${pattern}$`, "i").test(path);
}

interface FileListProps {
    items: gitdiff.FileChange[];
    selectedPath: string | null;
    checked: Set<string>;
    fileFilter: string;
    onFileFilterChange: (filter: string) => void;
    onSelectFile: (path: string) => void;
    onToggleChecked: (path: string) => void;
    onToggleCheckedAll: (items: gitdiff.FileChange[]) => void;
    historyMode: boolean;
}

function FileList(props: FileListProps) {
    const {
        items,
        selectedPath,
        checked,
        fileFilter,
        onFileFilterChange,
        onSelectFile,
        onToggleChecked,
        onToggleCheckedAll,
        historyMode,
    } = props;
    const visible = fileFilter ? items.filter((f) => matchesFilter(f.Path, fileFilter)) : items;
    const allChecked = visible.length > 0 && visible.every((f) => checked.has(f.Path));
    const someChecked = visible.some((f) => checked.has(f.Path));
    const totalAdditions = items.reduce((sum, f) => sum + f.Additions, 0);
    const totalDeletions = items.reduce((sum, f) => sum + f.Deletions, 0);
    return (
        <>
            {items.length > 0 && (
                <div className={historyMode ? "file-list-stat file-list-stat--history" : "file-list-stat"}>
                    <span className="stat-files">
                        <DocIcon /> {items.length}
                    </span>
                    <span className="stat-diff">
                        <span className="stat-added">+{totalAdditions}</span>
                        <span className="stat-deleted">−{totalDeletions}</span>
                    </span>
                </div>
            )}
            <div className="file-list-select-all">
                <input
                    type="checkbox"
                    checked={allChecked}
                    ref={(el) => {
                        if (el) {
                            el.indeterminate = !allChecked && someChecked;
                        }
                    }}
                    onChange={() => onToggleCheckedAll(visible)}
                />
                <input
                    type="text"
                    className="file-filter-input"
                    placeholder="filter…"
                    value={fileFilter}
                    onChange={(e) => onFileFilterChange(e.target.value)}
                />
            </div>
            <ul className="file-list">
                {visible.map((file) => {
                    const status = statusStyles[file.Type] ?? fallbackStatus;
                    return (
                        <li
                            key={file.Path}
                            className={file.Path === selectedPath ? "file-item selected" : "file-item"}
                            onClick={() => onSelectFile(file.Path)}
                        >
                            <input
                                type="checkbox"
                                className="file-checkbox"
                                checked={checked.has(file.Path)}
                                onClick={(e) => e.stopPropagation()}
                                onChange={() => onToggleChecked(file.Path)}
                            />
                            <span className={`file-type ${status.className}`} title={status.label}>
                                {status.glyph}
                            </span>
                            <span className="file-path">{file.Path}</span>
                        </li>
                    );
                })}
            </ul>
        </>
    );
}

export default FileList;
