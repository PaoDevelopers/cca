import { useEffect, useState, type FormEvent } from "react"

import { apiRequest, jsonBody } from "@/api"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
import { Button } from "@/components/ui/button"
import {
	Card,
	CardContent,
	CardDescription,
	CardFooter,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import {
	Field,
	FieldDescription,
	FieldError,
	FieldGroup,
	FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { Spinner } from "@/components/ui/spinner"

type TestRole = "student" | "admin"

interface TestAuthSettings {
	enabled: boolean
	requires_access_key: boolean
}

interface TestAuthResponse {
	redirect_to: "/student/" | "/admin/"
}

interface LoginCardProps {
	role: TestRole
	requiresAccessKey: boolean
}

const loginCardContent: Record<
	TestRole,
	{
		title: string
		description: string
		identifierLabel: string
		identifierPlaceholder: string
		buttonLabel: string
	}
> = {
	student: {
		title: "Student access",
		description: "Enter a student ID that exists in the current database.",
		identifierLabel: "Student ID",
		identifierPlaceholder: "e.g. S12345…",
		buttonLabel: "Continue as student",
	},
	admin: {
		title: "Administrator access",
		description:
			"Enter the administrator username configured on the server.",
		identifierLabel: "Admin username",
		identifierPlaceholder: "Configured admin username…",
		buttonLabel: "Continue as administrator",
	},
}

function LoginCard({
	role,
	requiresAccessKey,
}: LoginCardProps): React.JSX.Element {
	const content = loginCardContent[role]
	const [identifier, setIdentifier] = useState("")
	const [accessKey, setAccessKey] = useState("")
	const [pending, setPending] = useState(false)
	const [error, setError] = useState<string | null>(null)
	const identifierID = `${role}-identifier`
	const accessKeyID = `${role}-access-key`

	const handleSubmit = async (event: FormEvent<HTMLFormElement>) => {
		event.preventDefault()
		setPending(true)
		setError(null)
		try {
			const result = await apiRequest<TestAuthResponse>(
				"/api/v1/test-auth",
				{
					method: "POST",
					body: jsonBody({
						role,
						identifier: identifier.trim(),
						...(requiresAccessKey ? { access_key: accessKey } : {}),
					}),
				},
			)
			if (
				result.redirect_to !== "/student/" &&
				result.redirect_to !== "/admin/"
			) {
				throw new Error("The server returned an invalid redirect.")
			}
			window.location.assign(result.redirect_to)
		} catch (caught) {
			setError(
				caught instanceof Error
					? caught.message
					: "Test sign-in failed. Please check the identifier and try again.",
			)
			setPending(false)
		}
	}

	return (
		<form onSubmit={(event) => void handleSubmit(event)}>
			<Card className="h-full">
				<CardHeader>
					<CardTitle>{content.title}</CardTitle>
					<CardDescription>{content.description}</CardDescription>
				</CardHeader>
				<CardContent>
					<FieldGroup>
						<Field>
							<FieldLabel htmlFor={identifierID}>
								{content.identifierLabel}
							</FieldLabel>
							<Input
								id={identifierID}
								name="identifier"
								value={identifier}
								onChange={(event) =>
									setIdentifier(event.target.value)
								}
								placeholder={content.identifierPlaceholder}
								autoComplete="username"
								disabled={pending}
								required
							/>
							<FieldDescription>
								This value is checked against the server
								configuration and database.
							</FieldDescription>
						</Field>
						{requiresAccessKey ? (
							<Field>
								<FieldLabel htmlFor={accessKeyID}>
									Test access key
								</FieldLabel>
								<Input
									id={accessKeyID}
									name="access_key"
									type="password"
									value={accessKey}
									onChange={(event) =>
										setAccessKey(event.target.value)
									}
									autoComplete="current-password"
									disabled={pending}
									required
								/>
							</Field>
						) : null}
						{error !== null ? (
							<FieldError>{error}</FieldError>
						) : null}
					</FieldGroup>
				</CardContent>
				<CardFooter>
					<Button type="submit" className="w-full" disabled={pending}>
						{pending ? <Spinner data-icon="inline-start" /> : null}
						{pending ? "Signing in…" : content.buttonLabel}
					</Button>
				</CardFooter>
			</Card>
		</form>
	)
}

export default function TestLoginPage(): React.JSX.Element {
	const [settings, setSettings] = useState<TestAuthSettings | null>(null)
	const [error, setError] = useState<string | null>(null)

	useEffect(() => {
		let active = true
		void apiRequest<TestAuthSettings>("/api/v1/test-auth")
			.then((result) => {
				if (active) setSettings(result)
			})
			.catch((caught: unknown) => {
				if (!active) return
				setError(
					caught instanceof Error
						? caught.message
						: "Unable to load the test authentication settings.",
				)
			})
		return () => {
			active = false
		}
	}, [])

	return (
		<main
			id="main-content"
			className="flex min-h-svh justify-center p-4 sm:p-6 lg:p-10"
		>
			<div className="flex w-full max-w-5xl flex-col gap-6">
				<header className="flex flex-col gap-2">
					<p className="text-sm font-medium text-primary">
						CCA Sign-Up
					</p>
					<h1 className="text-2xl font-semibold tracking-tight sm:text-3xl">
						Test sign-in
					</h1>
					<p className="max-w-2xl text-muted-foreground">
						Choose a role to create a temporary application session
						without the normal identity provider.
					</p>
				</header>

				{error !== null ? (
					<Alert variant="destructive">
						<AlertTitle>Test sign-in is unavailable</AlertTitle>
						<AlertDescription>{error}</AlertDescription>
					</Alert>
				) : settings === null ? (
					<div
						className="flex min-h-48 items-center justify-center"
						aria-live="polite"
					>
						<Spinner />
					</div>
				) : !settings.enabled ? (
					<Alert>
						<AlertTitle>Test mode is disabled</AlertTitle>
						<AlertDescription>
							Enable test authentication in the server settings
							before using this page.
						</AlertDescription>
					</Alert>
				) : (
					<>
						<Alert variant="destructive">
							<AlertTitle>TEST MODE IS ACTIVE</AlertTitle>
							<AlertDescription>
								This page bypasses normal identity-provider
								authentication. Use it only in an isolated
								development or testing environment.
							</AlertDescription>
						</Alert>
						<div className="grid items-stretch gap-4 md:grid-cols-2">
							<LoginCard
								role="student"
								requiresAccessKey={settings.requires_access_key}
							/>
							<LoginCard
								role="admin"
								requiresAccessKey={settings.requires_access_key}
							/>
						</div>
					</>
				)}
			</div>
		</main>
	)
}
