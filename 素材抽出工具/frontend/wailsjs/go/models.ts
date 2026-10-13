export namespace applock {

	export class Status {
	    enabled: boolean;
	    failures: number;
	    lockedUntil: number;

	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.enabled = source["enabled"];
	        this.failures = source["failures"];
	        this.lockedUntil = source["lockedUntil"];
	    }
	}

}

export namespace library {
	
	export class PickPart {
	    listId: string;
	    name: string;
	    count: number;
	    repeat: number;
	    mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new PickPart(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.listId = source["listId"];
	        this.name = source["name"];
	        this.count = source["count"];
	        this.repeat = source["repeat"];
	        this.mode = source["mode"];
	    }
	}
	export class ExtractResult {
	    text: string;
	    total: number;
	    parts: PickPart[];
	
	    static createFrom(source: any = {}) {
	        return new ExtractResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.text = source["text"];
	        this.total = source["total"];
	        this.parts = this.convertValues(source["parts"], PickPart);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Group {
	    id: string;
	    name: string;
	    color: string;
	    collapsed: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Group(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	        this.collapsed = source["collapsed"];
	    }
	}
	export class HistoryEntry {
	    id: string;
	    time: number;
	    total: number;
	    parts: PickPart[];
	    text?: string;
	    truncated: boolean;
	
	    static createFrom(source: any = {}) {
	        return new HistoryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.time = source["time"];
	        this.total = source["total"];
	        this.parts = this.convertValues(source["parts"], PickPart);
	        this.text = source["text"];
	        this.truncated = source["truncated"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportItem {
	    source: string;
	    text: string;
	    mode: string;
	    targetId: string;
	    newName: string;
	    groupId: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.text = source["text"];
	        this.mode = source["mode"];
	        this.targetId = source["targetId"];
	        this.newName = source["newName"];
	        this.groupId = source["groupId"];
	    }
	}
	export class ImportPlan {
	    items: ImportItem[];
	    dedupe: boolean;
	
	    static createFrom(source: any = {}) {
	        return new ImportPlan(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ImportItem);
	        this.dedupe = source["dedupe"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class ImportStat {
	    source: string;
	    target: string;
	    mode: string;
	    lines: number;
	    added: number;
	    duplicates: number;
	    removed: number;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportStat(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.target = source["target"];
	        this.mode = source["mode"];
	        this.lines = source["lines"];
	        this.added = source["added"];
	        this.duplicates = source["duplicates"];
	        this.removed = source["removed"];
	        this.error = source["error"];
	    }
	}
	export class ImportReport {
	    items: ImportStat[];
	    added: number;
	    duplicates: number;
	    removed: number;
	    created: number;
	    touched: number;
	    ok: boolean;
	    error: string;
	
	    static createFrom(source: any = {}) {
	        return new ImportReport(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.items = this.convertValues(source["items"], ImportStat);
	        this.added = source["added"];
	        this.duplicates = source["duplicates"];
	        this.removed = source["removed"];
	        this.created = source["created"];
	        this.touched = source["touched"];
	        this.ok = source["ok"];
	        this.error = source["error"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	
	export class ListInfo {
	    id: string;
	    name: string;
	    groupId: string;
	    count: number;
	    unique: number;
	    updatedAt: number;
	    tagIds: string[];
	
	    static createFrom(source: any = {}) {
	        return new ListInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.groupId = source["groupId"];
	        this.count = source["count"];
	        this.unique = source["unique"];
	        this.updatedAt = source["updatedAt"];
	        this.tagIds = source["tagIds"];
	    }
	}
	export class Pick {
	    count: number;
	    repeat: number;
	    mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new Pick(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.count = source["count"];
	        this.repeat = source["repeat"];
	        this.mode = source["mode"];
	    }
	}
	
	export class PickRequest {
	    listId: string;
	    count: number;
	    repeat: number;
	    mode?: string;
	
	    static createFrom(source: any = {}) {
	        return new PickRequest(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.listId = source["listId"];
	        this.count = source["count"];
	        this.repeat = source["repeat"];
	        this.mode = source["mode"];
	    }
	}
	export class Preset {
	    id: string;
	    name: string;
	    picks: Record<string, Pick>;
	
	    static createFrom(source: any = {}) {
	        return new Preset(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.picks = this.convertValues(source["picks"], Pick, true);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Settings {
	    alwaysOnTop: boolean;
	    afterCopy: string;
	    rememberDraft: boolean;
	    theme: string;
	    importDedupe: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alwaysOnTop = source["alwaysOnTop"];
	        this.afterCopy = source["afterCopy"];
	        this.rememberDraft = source["rememberDraft"];
	        this.theme = source["theme"];
	        this.importDedupe = source["importDedupe"];
	    }
	}
	export class Tag {
	    id: string;
	    name: string;
	    color: string;

	    static createFrom(source: any = {}) {
	        return new Tag(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.color = source["color"];
	    }
	}
	export class Snapshot {
	    groups: Group[];
	    tags: Tag[];
	    lists: ListInfo[];
	    presets: Preset[];
	    draft: Record<string, Pick>;
	    settings: Settings;
	    undo: string;
	
	    static createFrom(source: any = {}) {
	        return new Snapshot(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.groups = this.convertValues(source["groups"], Group);
	        this.tags = this.convertValues(source["tags"], Tag);
	        this.lists = this.convertValues(source["lists"], ListInfo);
	        this.presets = this.convertValues(source["presets"], Preset);
	        this.draft = this.convertValues(source["draft"], Pick, true);
	        this.settings = this.convertValues(source["settings"], Settings);
	        this.undo = source["undo"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}

export namespace license {
	
	export class Status {
	    state: string;
	    key: string;
	    masked: string;
	    expiry: string;
	    offline: boolean;
	    graceUntil: number;
	    code: string;
	    message: string;
	    network: boolean;
	    machine: string;
	
	    static createFrom(source: any = {}) {
	        return new Status(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = source["state"];
	        this.key = source["key"];
	        this.masked = source["masked"];
	        this.expiry = source["expiry"];
	        this.offline = source["offline"];
	        this.graceUntil = source["graceUntil"];
	        this.code = source["code"];
	        this.message = source["message"];
	        this.network = source["network"];
	        this.machine = source["machine"];
	    }
	}

}

export namespace main {
	
	export class Created {
	    snapshot: library.Snapshot;
	    id: string;
	
	    static createFrom(source: any = {}) {
	        return new Created(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapshot = this.convertValues(source["snapshot"], library.Snapshot);
	        this.id = source["id"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Deduped {
	    snapshot: library.Snapshot;
	    removed: number;
	
	    static createFrom(source: any = {}) {
	        return new Deduped(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapshot = this.convertValues(source["snapshot"], library.Snapshot);
	        this.removed = source["removed"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Extracted {
	    result: library.ExtractResult;
	    copied: boolean;
	    historyId: string;
	    time: number;
	
	    static createFrom(source: any = {}) {
	        return new Extracted(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.result = this.convertValues(source["result"], library.ExtractResult);
	        this.copied = source["copied"];
	        this.historyId = source["historyId"];
	        this.time = source["time"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Imported {
	    snapshot: library.Snapshot;
	    report: library.ImportReport;
	
	    static createFrom(source: any = {}) {
	        return new Imported(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapshot = this.convertValues(source["snapshot"], library.Snapshot);
	        this.report = this.convertValues(source["report"], library.ImportReport);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class InitInfo {
	    snapshot: library.Snapshot;
	    notice: string;
	    version: string;
	    platform: string;
	    maxLines: number;
	    lock: applock.Status;
	
	    static createFrom(source: any = {}) {
	        return new InitInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapshot = this.convertValues(source["snapshot"], library.Snapshot);
	        this.notice = source["notice"];
	        this.version = source["version"];
	        this.platform = source["platform"];
	        this.maxLines = source["maxLines"];
	        this.lock = this.convertValues(source["lock"], applock.Status);
	    }

		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class LockResult {
	    ok: boolean;
	    status: applock.Status;

	    static createFrom(source: any = {}) {
	        return new LockResult(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ok = source["ok"];
	        this.status = this.convertValues(source["status"], applock.Status);
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}
	export class Restored {
	    snapshot: library.Snapshot;
	    restored: boolean;
	
	    static createFrom(source: any = {}) {
	        return new Restored(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.snapshot = this.convertValues(source["snapshot"], library.Snapshot);
	        this.restored = source["restored"];
	    }
	
		convertValues(a: any, classs: any, asMap: boolean = false): any {
		    if (!a) {
		        return a;
		    }
		    if (a.slice && a.map) {
		        return (a as any[]).map(elem => this.convertValues(elem, classs));
		    } else if ("object" === typeof a) {
		        if (asMap) {
		            for (const key of Object.keys(a)) {
		                a[key] = new classs(a[key]);
		            }
		            return a;
		        }
		        return new classs(a);
		    }
		    return a;
		}
	}

}
