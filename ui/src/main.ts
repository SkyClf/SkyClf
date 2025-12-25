import { createApp } from "vue";
import "./style.css";
import Root from "./Root.vue";
import router from "./router";

createApp(Root).use(router).mount("#app");
