export namespace config {
	
	export class BBox {
	    x: number;
	    y: number;
	    width: number;
	    height: number;
	
	    static createFrom(source: any = {}) {
	        return new BBox(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.x = source["x"];
	        this.y = source["y"];
	        this.width = source["width"];
	        this.height = source["height"];
	    }
	}
	export class DialogueItem {
	    id: number;
	    frameIndex: number;
	    timestamp: number;
	    duration?: number;
	    endTimestamp?: number;
	    original: string;
	    translated: string;
	    translationFailed: boolean;
	    isTrash?: boolean;
	    bbox: BBox;
	
	    static createFrom(source: any = {}) {
	        return new DialogueItem(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.frameIndex = source["frameIndex"];
	        this.timestamp = source["timestamp"];
	        this.duration = source["duration"];
	        this.endTimestamp = source["endTimestamp"];
	        this.original = source["original"];
	        this.translated = source["translated"];
	        this.translationFailed = source["translationFailed"];
	        this.isTrash = source["isTrash"];
	        this.bbox = this.convertValues(source["bbox"], BBox);
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
	export class HealthStatus {
	    ocrOnline: boolean;
	    ocrEngine: string;
	    hasApiKey: boolean;
	    errorMsg: string;
	
	    static createFrom(source: any = {}) {
	        return new HealthStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ocrOnline = source["ocrOnline"];
	        this.ocrEngine = source["ocrEngine"];
	        this.hasApiKey = source["hasApiKey"];
	        this.errorMsg = source["errorMsg"];
	    }
	}
	export class ScanConfig {
	    videoPath: string;
	    interval: number;
	    threshold: number;
	    sourceLang: string;
	    geminiKey: string;
	    detectorRevision: number;
	    scanMode: string;
	
	    static createFrom(source: any = {}) {
	        return new ScanConfig(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.videoPath = source["videoPath"];
	        this.interval = source["interval"];
	        this.threshold = source["threshold"];
	        this.sourceLang = source["sourceLang"];
	        this.geminiKey = source["geminiKey"];
	        this.detectorRevision = source["detectorRevision"];
	        this.scanMode = source["scanMode"];
	    }
	}

}

export namespace translation {
	
	export class GlossaryEntry {
	    source: string;
	    target: string;
	
	    static createFrom(source: any = {}) {
	        return new GlossaryEntry(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.source = source["source"];
	        this.target = source["target"];
	    }
	}

}

