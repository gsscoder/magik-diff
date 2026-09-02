export namespace checks {
	
	export class Check {
	    Name: string;
	    Description: string;
	    Color: string;
	    Prompt: string;
	
	    static createFrom(source: any = {}) {
	        return new Check(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Name = source["Name"];
	        this.Description = source["Description"];
	        this.Color = source["Color"];
	        this.Prompt = source["Prompt"];
	    }
	}

}

export namespace config {
	
	export class Config {
	    base_url: string;
	    model: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.base_url = source["base_url"];
	        this.model = source["model"];
	    }
	}

}

export namespace diffparse {
	
	export class Span {
	    Start: number;
	    End: number;
	
	    static createFrom(source: any = {}) {
	        return new Span(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Start = source["Start"];
	        this.End = source["End"];
	    }
	}
	export class Line {
	    Type: string;
	    Content: string;
	    OldNum: number;
	    NewNum: number;
	    Highlight: Span;
	
	    static createFrom(source: any = {}) {
	        return new Line(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Type = source["Type"];
	        this.Content = source["Content"];
	        this.OldNum = source["OldNum"];
	        this.NewNum = source["NewNum"];
	        this.Highlight = this.convertValues(source["Highlight"], Span);
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
	export class Hunk {
	    Header: string;
	    Lines: Line[];
	
	    static createFrom(source: any = {}) {
	        return new Hunk(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Header = source["Header"];
	        this.Lines = this.convertValues(source["Lines"], Line);
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
	export class FileDiff {
	    Path: string;
	    Hunks: Hunk[];
	
	    static createFrom(source: any = {}) {
	        return new FileDiff(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.Hunks = this.convertValues(source["Hunks"], Hunk);
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

export namespace gitdiff {
	
	export class Commit {
	    Hash: string;
	    Author: string;
	    Date: string;
	    Subject: string;
	
	    static createFrom(source: any = {}) {
	        return new Commit(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Hash = source["Hash"];
	        this.Author = source["Author"];
	        this.Date = source["Date"];
	        this.Subject = source["Subject"];
	    }
	}
	export class FileChange {
	    Path: string;
	    OrigPath: string;
	    Type: string;
	    IsCode: boolean;
	    Additions: number;
	    Deletions: number;
	
	    static createFrom(source: any = {}) {
	        return new FileChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.OrigPath = source["OrigPath"];
	        this.Type = source["Type"];
	        this.IsCode = source["IsCode"];
	        this.Additions = source["Additions"];
	        this.Deletions = source["Deletions"];
	    }
	}

}

export namespace main {
	
	export class OpenFolderResult {
	    Canceled: boolean;
	    Path: string;
	    Valid: boolean;
	
	    static createFrom(source: any = {}) {
	        return new OpenFolderResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Canceled = source["Canceled"];
	        this.Path = source["Path"];
	        this.Valid = source["Valid"];
	    }
	}

}

