import { BrowserRouter } from "react-router";
import AppRouter from "./router/AppRouter";
import { AuthProvider } from "./auth/AuthProvider";

export default function App() {
  return (
    <AuthProvider>
      <BrowserRouter>
        <AppRouter />
      </BrowserRouter>
    </AuthProvider>
  );
}
