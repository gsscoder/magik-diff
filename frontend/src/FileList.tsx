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

interface FileListProps {
    items: gitdiff.FileChange[];
    selectedPath: string | null;
    checked: Set<string>;
    fileFilter: string;
    onFileFilterChange: (filter: string) => void;
    onSelectFile: (path: string) => void;
    onToggleChecked: (path: string) => void;
    onToggleCheckedAll: (items: gitdiff.FileChange[]) => void;
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
    } = props;
    const needle = fileFilter.toLowerCase();
    const visible = needle ? items.filter((f) => f.Path.toLowerCase().includes(needle)) : items;
    const allChecked = visible.length > 0 && visible.every((f) => checked.has(f.Path));
    const someChecked = visible.some((f) => checked.has(f.Path));
    return (
        <>
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
