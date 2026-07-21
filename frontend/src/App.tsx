import { BrowserRouter } from "react-router";
import AppRouter from "./router/AppRouter";

export default function App() {
  return (
    <BrowserRouter>
      <AppRouter />
    </BrowserRouter>
  );
}
