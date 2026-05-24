export namespace config {
	
	export class ModelMapping {
	    from: string;
	    to: string;
	
	    static createFrom(source: any = {}) {
	        return new ModelMapping(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.from = source["from"];
	        this.to = source["to"];
	    }
	}
	export class Provider {
	    id: string;
	    name: string;
	    base_url: string;
	    api_key: string;
	    default_model: string;
	    model_mappings: ModelMapping[];
	    cli_types: string[];
	    enabled: boolean;
	    created_at: number;
	    prompt_tokens: number;
	    completion_tokens: number;
	    total_tokens: number;
	    usage_updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new Provider(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.name = source["name"];
	        this.base_url = source["base_url"];
	        this.api_key = source["api_key"];
	        this.default_model = source["default_model"];
	        this.model_mappings = this.convertValues(source["model_mappings"], ModelMapping);
	        this.cli_types = source["cli_types"];
	        this.enabled = source["enabled"];
	        this.created_at = source["created_at"];
	        this.prompt_tokens = source["prompt_tokens"];
	        this.completion_tokens = source["completion_tokens"];
	        this.total_tokens = source["total_tokens"];
	        this.usage_updated_at = source["usage_updated_at"];
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
	export class AppSettings {
	    port: number;
	    providers: Provider[];
	
	    static createFrom(source: any = {}) {
	        return new AppSettings(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.port = source["port"];
	        this.providers = this.convertValues(source["providers"], Provider);
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

export namespace main {
	
	export class ProxyStatus {
	    running: boolean;
	    port: number;
	    addr: string;
	
	    static createFrom(source: any = {}) {
	        return new ProxyStatus(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.running = source["running"];
	        this.port = source["port"];
	        this.addr = source["addr"];
	    }
	}

}

export namespace proxy {
	
	export class ProviderUsagePoint {
	    time: number;
	    prompt_tokens: number;
	    completion_tokens: number;
	    total_tokens: number;
	
	    static createFrom(source: any = {}) {
	        return new ProviderUsagePoint(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.time = source["time"];
	        this.prompt_tokens = source["prompt_tokens"];
	        this.completion_tokens = source["completion_tokens"];
	        this.total_tokens = source["total_tokens"];
	    }
	}
	export class ProviderUsageStats {
	    provider_id: string;
	    provider: string;
	    prompt_tokens: number;
	    completion_tokens: number;
	    total_tokens: number;
	    updated_at: number;
	
	    static createFrom(source: any = {}) {
	        return new ProviderUsageStats(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.provider_id = source["provider_id"];
	        this.provider = source["provider"];
	        this.prompt_tokens = source["prompt_tokens"];
	        this.completion_tokens = source["completion_tokens"];
	        this.total_tokens = source["total_tokens"];
	        this.updated_at = source["updated_at"];
	    }
	}
	export class RequestLog {
	    id: string;
	    time: number;
	    method: string;
	    path: string;
	    cli_type: string;
	    provider_id?: string;
	    provider: string;
	    model: string;
	    status_code: number;
	    duration_ms: number;
	    prompt_tokens: number;
	    completion_tokens: number;
	    total_tokens: number;
	    error?: string;
	    response_body?: string;
	
	    static createFrom(source: any = {}) {
	        return new RequestLog(source);
	    }
	
	    constructor(source: any = {}) {
	        if ('string' === typeof source) source = JSON.parse(source);
	        this.id = source["id"];
	        this.time = source["time"];
	        this.method = source["method"];
	        this.path = source["path"];
	        this.cli_type = source["cli_type"];
	        this.provider_id = source["provider_id"];
	        this.provider = source["provider"];
	        this.model = source["model"];
	        this.status_code = source["status_code"];
	        this.duration_ms = source["duration_ms"];
	        this.prompt_tokens = source["prompt_tokens"];
	        this.completion_tokens = source["completion_tokens"];
	        this.total_tokens = source["total_tokens"];
	        this.error = source["error"];
	        this.response_body = source["response_body"];
	    }
	}

}

