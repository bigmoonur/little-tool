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

	export class InitInfo {
	    settings: picker.Settings;
	    version: string;
	    platform: string;

	    static createFrom(source: any = {}) {
	        return new InitInfo(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.settings = this.convertValues(source["settings"], picker.Settings);
	        this.version = source["version"];
	        this.platform = source["platform"];
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
	export class PlanSummary {
	    id: string;
	    count: number;
	    bytes: number;
	    old: number;
	    destination: string;
	    folder: string;
	    items: picker.Item[];

	    static createFrom(source: any = {}) {
	        return new PlanSummary(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.count = source["count"];
	        this.bytes = source["bytes"];
	        this.old = source["old"];
	        this.destination = source["destination"];
	        this.folder = source["folder"];
	        this.items = this.convertValues(source["items"], picker.Item);
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
	export class State {
	    materialRoot: string;
	    outputRoot: string;
	    folder: string;
	    categories: picker.Category[];
	    tags: picker.Tag[];
	    destinations: picker.Destination[];
	    materialError: string;
	    outputError: string;
	    leftovers: string[];
	    busy: boolean;

	    static createFrom(source: any = {}) {
	        return new State(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.materialRoot = source["materialRoot"];
	        this.outputRoot = source["outputRoot"];
	        this.folder = source["folder"];
	        this.categories = this.convertValues(source["categories"], picker.Category);
	        this.tags = this.convertValues(source["tags"], picker.Tag);
	        this.destinations = this.convertValues(source["destinations"], picker.Destination);
	        this.materialError = source["materialError"];
	        this.outputError = source["outputError"];
	        this.leftovers = source["leftovers"];
	        this.busy = source["busy"];
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
	export class TagCreated {
	    state: State;
	    id: string;

	    static createFrom(source: any = {}) {
	        return new TagCreated(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.state = this.convertValues(source["state"], State);
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
	export class UISettings {
	    alwaysOnTop: boolean;
	    openAfter: boolean;
	    theme: string;
	    rememberDraft: boolean;

	    static createFrom(source: any = {}) {
	        return new UISettings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.alwaysOnTop = source["alwaysOnTop"];
	        this.openAfter = source["openAfter"];
	        this.theme = source["theme"];
	        this.rememberDraft = source["rememberDraft"];
	    }
	}

}

export namespace picker {

	export class Category {
	    name: string;
	    count: number;
	    bytes: number;
	    tagIds: string[];

	    static createFrom(source: any = {}) {
	        return new Category(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.count = source["count"];
	        this.bytes = source["bytes"];
	        this.tagIds = source["tagIds"];
	    }
	}
	export class Destination {
	    name: string;
	    videos: number;

	    static createFrom(source: any = {}) {
	        return new Destination(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.name = source["name"];
	        this.videos = source["videos"];
	    }
	}
	export class Item {
	    category: string;
	    name: string;
	    size: number;

	    static createFrom(source: any = {}) {
	        return new Item(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.category = source["category"];
	        this.name = source["name"];
	        this.size = source["size"];
	    }
	}
	export class HistoryEntry {
	    id: string;
	    time: number;
	    destination: string;
	    folder: string;
	    items: Item[];
	    removed: number;
	    bytes: number;

	    static createFrom(source: any = {}) {
	        return new HistoryEntry(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.time = source["time"];
	        this.destination = source["destination"];
	        this.folder = source["folder"];
	        this.items = this.convertValues(source["items"], Item);
	        this.removed = source["removed"];
	        this.bytes = source["bytes"];
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
	    materialRoot: string;
	    outputRoot: string;
	    folder: string;
	    alwaysOnTop: boolean;
	    openAfter: boolean;
	    theme: string;
	    rememberDraft: boolean;
	    draft: Record<string, number>;

	    static createFrom(source: any = {}) {
	        return new Settings(source);
	    }

	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.materialRoot = source["materialRoot"];
	        this.outputRoot = source["outputRoot"];
	        this.folder = source["folder"];
	        this.alwaysOnTop = source["alwaysOnTop"];
	        this.openAfter = source["openAfter"];
	        this.theme = source["theme"];
	        this.rememberDraft = source["rememberDraft"];
	        this.draft = source["draft"];
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

}
