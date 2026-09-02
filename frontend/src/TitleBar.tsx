import {useEffect, useRef, useState} from "react";
import {Quit, WindowIsMaximised, WindowMinimise, WindowToggleMaximise} from "../wailsjs/runtime/runtime";

function MinimiseIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M1 5H9" stroke="currentColor" />
        </svg>
    );
}

function MaximiseIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <rect x="1.5" y="1.5" width="7" height="7" stroke="currentColor" />
        </svg>
    );
}

function RestoreIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M3 1H9V7" stroke="currentColor" />
            <rect x="1" y="3" width="6" height="6" stroke="currentColor" />
        </svg>
    );
}

function CloseIcon() {
    return (
        <svg width="10" height="10" viewBox="0 0 10 10" fill="none" aria-hidden="true">
            <path d="M1.5 1.5L8.5 8.5M8.5 1.5L1.5 8.5" stroke="currentColor" />
        </svg>
    );
}

function SidebarIcon({visible}: {visible: boolean}) {
    return (
        <svg width="14" height="14" viewBox="0 0 16 16" fill="none" aria-hidden="true">
            <rect x="1.5" y="2.5" width="13" height="11" rx="1.5" stroke="currentColor" />
            <path d="M6 2.5V13.5" stroke="currentColor" />
            {visible && <rect x="2.5" y="3.5" width="3" height="9" fill="currentColor" />}
        </svg>
    );
}

function GearIcon() {
    return (
        <svg width="14" height="14" viewBox="0 0 24 24" fill="none" aria-hidden="true">
            <circle cx="12" cy="12" r="3" stroke="currentColor" strokeWidth="2" />
            <path
                d="M19.4 15a1.65 1.65 0 0 0 .33 1.82l.06.06a2 2 0 1 1-2.83 2.83l-.06-.06a1.65 1.65 0 0 0-1.82-.33 1.65 1.65 0 0 0-1 1.51V21a2 2 0 0 1-4 0v-.09A1.65 1.65 0 0 0 9 19.4a1.65 1.65 0 0 0-1.82.33l-.06.06a2 2 0 1 1-2.83-2.83l.06-.06a1.65 1.65 0 0 0 .33-1.82 1.65 1.65 0 0 0-1.51-1H3a2 2 0 0 1 0-4h.09A1.65 1.65 0 0 0 4.6 9a1.65 1.65 0 0 0-.33-1.82l-.06-.06a2 2 0 1 1 2.83-2.83l.06.06a1.65 1.65 0 0 0 1.82.33H9a1.65 1.65 0 0 0 1-1.51V3a2 2 0 0 1 4 0v.09a1.65 1.65 0 0 0 1 1.51 1.65 1.65 0 0 0 1.82-.33l.06-.06a2 2 0 1 1 2.83 2.83l-.06.06a1.65 1.65 0 0 0-.33 1.82V9a1.65 1.65 0 0 0 1.51 1H21a2 2 0 0 1 0 4h-.09a1.65 1.65 0 0 0-1.51 1z"
                stroke="currentColor"
                strokeWidth="2"
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}

type TitleBarProps = {
    onOpenRepo: () => void;
    onOpenModelConfig: () => void;
    railVisible: boolean;
    onToggleRailVisible: () => void;
};

declare global {
    interface Window {
        wails?: {flags: {enableResize: boolean; resizeEdge?: string}};
    }
}

function TitleBar({onOpenRepo, onOpenModelConfig, railVisible, onToggleRailVisible}: TitleBarProps) {
    const [maximised, setMaximised] = useState(false);
    const [menuOpen, setMenuOpen] = useState(false);
    const resizeFrame = useRef(0);
    const menuRef = useRef<HTMLDivElement>(null);

    useEffect(() => {
        function poll() {
            WindowIsMaximised().then(setMaximised);
        }
        poll();
        // Wails v2.13 fires no window-state-changed event, so Aero-snap and
        // Win+Up-arrow maximise/restore are only visible via the resize event
        // they trigger; coalesce with rAF because resize fires rapidly during
        // a live drag.
        function onResize() {
            cancelAnimationFrame(resizeFrame.current);
            resizeFrame.current = requestAnimationFrame(poll);
        }
        window.addEventListener("resize", onResize);
        return () => {
            window.removeEventListener("resize", onResize);
            cancelAnimationFrame(resizeFrame.current);
        };
    }, []);

    useEffect(() => {
        // Frameless Wails reserves a 6px band inside every window edge as a
        // resize handle and swallows its mousedown, which breaks clicks on
        // the window buttons and shrinks the drag strip. A maximised window
        // cannot be resized anyway, so disable the band there.
        const flags = window.wails?.flags;
        if (!flags) {
            return;
        }
        flags.enableResize = !maximised;
        if (maximised) {
            flags.resizeEdge = undefined;
            document.documentElement.style.cursor = "";
        }
    }, [maximised]);

    useEffect(() => {
        if (!menuOpen) {
            return;
        }
        function onClickOutside(e: MouseEvent) {
            if (menuRef.current && !menuRef.current.contains(e.target as Node)) {
                setMenuOpen(false);
            }
        }
        function onKeyDown(e: KeyboardEvent) {
            if (e.key === "Escape") {
                setMenuOpen(false);
            }
        }
        document.addEventListener("mousedown", onClickOutside);
        document.addEventListener("keydown", onKeyDown);
        return () => {
            document.removeEventListener("mousedown", onClickOutside);
            document.removeEventListener("keydown", onKeyDown);
        };
    }, [menuOpen]);

    return (
        <div className="titlebar">
            <div className="titlebar-drag" onDoubleClick={() => WindowToggleMaximise()}>
                <button
                    type="button"
                    className="titlebar-button titlebar-rail-toggle"
                    aria-label={railVisible ? "Hide changes/history panel" : "Show changes/history panel"}
                    title={railVisible ? "Hide changes/history panel" : "Show changes/history panel"}
                    aria-pressed={railVisible}
                    onClick={onToggleRailVisible}
                    onDoubleClick={(e) => e.stopPropagation()}
                >
                    <SidebarIcon visible={railVisible} />
                </button>
                <span className="titlebar-spacer" />
            </div>
            <div className="titlebar-menu" ref={menuRef}>
                <button
                    type="button"
                    className="titlebar-menu-button"
                    aria-label="Options menu"
                    title="Options"
                    aria-haspopup="true"
                    aria-expanded={menuOpen}
                    onClick={() => setMenuOpen((open) => !open)}
                >
                    <GearIcon />
                </button>
                {menuOpen && (
                    <div className="titlebar-menu-dropdown">
                        <button
                            type="button"
                            className="titlebar-menu-item"
                            onClick={() => {
                                setMenuOpen(false);
                                onOpenRepo();
                            }}
                        >
                            Open repository ...
                        </button>
                        <div className="titlebar-menu-separator" />
                        <button
                            type="button"
                            className="titlebar-menu-item"
                            onClick={() => {
                                setMenuOpen(false);
                                onOpenModelConfig();
                            }}
                        >
                            <span>Model config</span>
                            <span className="titlebar-menu-shortcut">Ctrl+M</span>
                        </button>
                    </div>
                )}
            </div>
            <div className="titlebar-controls">
                <button
                    type="button"
                    className="titlebar-button"
                    aria-label="Minimise"
                    title="Minimise"
                    onClick={() => WindowMinimise()}
                >
                    <MinimiseIcon />
                </button>
                <button
                    type="button"
                    className="titlebar-button"
                    aria-label={maximised ? "Restore" : "Maximise"}
                    title={maximised ? "Restore" : "Maximise"}
                    onClick={() => WindowToggleMaximise()}
                >
                    {maximised ? <RestoreIcon /> : <MaximiseIcon />}
                </button>
                <button
                    type="button"
                    className="titlebar-button titlebar-close"
                    aria-label="Close"
                    title="Close"
                    onClick={() => Quit()}
                >
                    <CloseIcon />
                </button>
            </div>
        </div>
    );
}

export default TitleBar;
