export class APIRequestError extends Error {
	public readonly status: number
	public readonly code: string

	public constructor(status: number, code: string, message: string) {
		super(message)
		this.name = "APIRequestError"
		this.status = status
		this.code = code
	}
}
