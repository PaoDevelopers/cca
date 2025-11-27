import "./app.css"
import App from "./App.svelte"
import { mount } from "svelte"

const target = document.getElementById("app")

if (!target) {
	throw new Error("App mount point missing")
}

mount(App, {
	target,
})
