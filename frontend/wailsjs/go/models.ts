export namespace api {
	
	export class MetadataResult {
	    title: string;
	    airingStatus: string;
	    totalEpisodes: number;
	    source: string;
	    nextEpisodeNum: number;
	    nextAiringAt: number;
	    score: number;
	    broadcastDay: string;
	
	    static createFrom(source: any = {}) {
	        return new MetadataResult(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.airingStatus = source["airingStatus"];
	        this.totalEpisodes = source["totalEpisodes"];
	        this.source = source["source"];
	        this.nextEpisodeNum = source["nextEpisodeNum"];
	        this.nextAiringAt = source["nextAiringAt"];
	        this.score = source["score"];
	        this.broadcastDay = source["broadcastDay"];
	    }
	}

}

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
	    domainAnikoto: string;
	    domainAnimeHeaven: string;
	    provider: string;
	    hlsTranscode: boolean;
	    browser: string;
	    browserPath: string;
	    minimizeToTray: boolean;
	    enableBackgroundMonitor: boolean;
	    autoDownloadTracked: boolean;
	    pollIntervalMinutes: number;
	    autoCheckUpdates: boolean;
	
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
	        this.domainAnikoto = source["domainAnikoto"];
	        this.domainAnimeHeaven = source["domainAnimeHeaven"];
	        this.provider = source["provider"];
	        this.hlsTranscode = source["hlsTranscode"];
	        this.browser = source["browser"];
	        this.browserPath = source["browserPath"];
	        this.minimizeToTray = source["minimizeToTray"];
	        this.enableBackgroundMonitor = source["enableBackgroundMonitor"];
	        this.autoDownloadTracked = source["autoDownloadTracked"];
	        this.pollIntervalMinutes = source["pollIntervalMinutes"];
	        this.autoCheckUpdates = source["autoCheckUpdates"];
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

export namespace tracker {
	
	export class TrackedAnime {
	    title: string;
	    slug: string;
	    poster: string;
	    lastDownloadedEp: number;
	    totalEpisodes: number;
	    airingStatus: string;
	    autoDownload: boolean;
	    nextEpisodeNum: number;
	    nextAiringAt: number;
	    score: number;
	    broadcastDay: string;
	    createdAt: string;
	    lastCheckedAt: string;
	
	    static createFrom(source: any = {}) {
	        return new TrackedAnime(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.title = source["title"];
	        this.slug = source["slug"];
	        this.poster = source["poster"];
	        this.lastDownloadedEp = source["lastDownloadedEp"];
	        this.totalEpisodes = source["totalEpisodes"];
	        this.airingStatus = source["airingStatus"];
	        this.autoDownload = source["autoDownload"];
	        this.nextEpisodeNum = source["nextEpisodeNum"];
	        this.nextAiringAt = source["nextAiringAt"];
	        this.score = source["score"];
	        this.broadcastDay = source["broadcastDay"];
	        this.createdAt = source["createdAt"];
	        this.lastCheckedAt = source["lastCheckedAt"];
	    }
	}

}

export namespace updater {
	
	export class UpdateInfo {
	    available: boolean;
	    currentVersion: string;
	    latestVersion: string;
	    releaseNotes: string;
	    downloadUrl: string;
	
	    static createFrom(source: any = {}) {
	        return new UpdateInfo(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.available = source["available"];
	        this.currentVersion = source["currentVersion"];
	        this.latestVersion = source["latestVersion"];
	        this.releaseNotes = source["releaseNotes"];
	        this.downloadUrl = source["downloadUrl"];
	    }
	}

}

