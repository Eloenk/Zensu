export namespace config {
	
	export class Config {
	    ua: string;
	    cf: string;
	    cookies: string;
	    downloadDir: string;
	    maxParallel: number;
	    quality: string;
	    audio: string;
	    domain: string;
	
	    static createFrom(source: any = {}) {
	        return new Config(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.ua = source["ua"];
	        this.cf = source["cf"];
	        this.cookies = source["cookies"];
	        this.downloadDir = source["downloadDir"];
	        this.maxParallel = source["maxParallel"];
	        this.quality = source["quality"];
	        this.audio = source["audio"];
	        this.domain = source["domain"];
	    }
	}

}

export namespace dl {
	
	export class JobProgress {
	    id: string;
	    anime: string;
	    epNum: number;
	    status: string;
	    progress: number;
	    speed: string;
	    eta: string;
	    error?: string;
	
	    static createFrom(source: any = {}) {
	        return new JobProgress(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.anime = source["anime"];
	        this.epNum = source["epNum"];
	        this.status = source["status"];
	        this.progress = source["progress"];
	        this.speed = source["speed"];
	        this.eta = source["eta"];
	        this.error = source["error"];
	    }
	}

}

export namespace main {
	
	export class AnimeResult {
	    session: string;
	    title: string;
	    poster: string;
	
	    static createFrom(source: any = {}) {
	        return new AnimeResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.session = source["session"];
	        this.title = source["title"];
	        this.poster = source["poster"];
	    }
	}
	export class EpisodeInfo {
	    episode: number;
	    session: string;
	    exists: boolean;
	
	    static createFrom(source: any = {}) {
	        return new EpisodeInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.episode = source["episode"];
	        this.session = source["session"];
	        this.exists = source["exists"];
	    }
	}

}

