import "./index.css"
import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { App } from "./App"

const target = document.getElementById("app")

if (target === null) {
	throw new Error("App mount point missing")
}

createRoot(target).render(
	<StrictMode>
		<App />
	</StrictMode>,
)
