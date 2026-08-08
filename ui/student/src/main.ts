import "../../common/styles/base.css"
import { mount } from "svelte"
import App from "./App.svelte"

const target = document.getElementById("app")

if (!target) {
	throw new Error("App mount point missing")
}

mount(App, {
	target,
})
