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
	
	    static createFrom(source: any = {}) {
	        return new FileChange(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.Path = source["Path"];
	        this.OrigPath = source["OrigPath"];
	        this.Type = source["Type"];
	    }
	}

}

