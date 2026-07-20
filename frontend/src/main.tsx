import { StrictMode } from "react"
import { createRoot } from "react-dom/client"
import { BrowserRouter } from "react-router-dom"
import { ThemeProvider } from "next-themes"

import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/sonner"
import App from "@/App"
import "@/style.css"

const root = document.getElementById("app")
if (root === null) throw new Error("Missing application root")

createRoot(root).render(
	<StrictMode>
		<ThemeProvider attribute="class" forcedTheme="light">
			<TooltipProvider>
				<BrowserRouter>
					<App />
				</BrowserRouter>
				<Toaster richColors position="top-center" />
			</TooltipProvider>
		</ThemeProvider>
	</StrictMode>,
)
