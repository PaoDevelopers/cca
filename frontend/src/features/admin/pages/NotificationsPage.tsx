import { useState } from "react"

import { adminRequest, jsonBody } from "@/api"
import { Button } from "@/components/ui/button"
import {
	Card,
	CardContent,
	CardDescription,
	CardHeader,
	CardTitle,
} from "@/components/ui/card"
import {
	Field,
	FieldDescription,
	FieldGroup,
	FieldLabel,
} from "@/components/ui/field"
import { Spinner } from "@/components/ui/spinner"
import { Textarea } from "@/components/ui/textarea"
import type { AdminPageProps } from "@/features/admin/app/AdminApp"
import { PageHeading } from "@/features/admin/components/AdminPagePrimitives"
import { runMutation } from "@/features/admin/lib/page-utils"

export function NotificationsPage({
	refresh,
}: AdminPageProps): React.JSX.Element {
	const [message, setMessage] = useState("")
	const [busy, setBusy] = useState(false)

	async function send(
		event: React.FormEvent<HTMLFormElement>,
	): Promise<void> {
		event.preventDefault()
		setBusy(true)
		const sent = await runMutation(
			() =>
				adminRequest("/notifications", {
					method: "POST",
					body: jsonBody({ message }),
				}),
			refresh,
			"Notification sent to connected students.",
		)
		setBusy(false)
		if (sent) setMessage("")
	}

	return (
		<>
			<PageHeading
				title="Notifications"
				description="Broadcast a short real-time message to currently connected students."
			/>
			<Card className="max-w-2xl">
				<CardHeader>
					<CardTitle>Broadcast message</CardTitle>
					<CardDescription>
						Messages are delivered live and are not stored as an
						inbox.
					</CardDescription>
				</CardHeader>
				<CardContent>
					<form onSubmit={(event) => void send(event)}>
						<FieldGroup>
							<Field>
								<FieldLabel htmlFor="notification-message">
									Message
								</FieldLabel>
								<Textarea
									id="notification-message"
									value={message}
									maxLength={1000}
									rows={6}
									onChange={(event) =>
										setMessage(event.target.value)
									}
									placeholder="Type an announcement for all connected users…"
									required
								/>
								<FieldDescription>
									{message.length}/1000 characters
								</FieldDescription>
							</Field>
							<Button
								type="submit"
								disabled={busy || message.trim() === ""}
							>
								{busy ? (
									<Spinner data-icon="inline-start" />
								) : null}
								Send notification
							</Button>
						</FieldGroup>
					</form>
				</CardContent>
			</Card>
		</>
	)
}
