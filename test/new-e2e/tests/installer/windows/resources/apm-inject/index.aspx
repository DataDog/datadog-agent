<%@ Page Language="C#" %>
<%@ Import Namespace="System" %>
<script runat="server">
    protected void Page_Load(object sender, EventArgs e)
    {
        Response.ContentType = "text/plain";
        string tracerHome = Environment.GetEnvironmentVariable("DD_DOTNET_TRACER_HOME");

        if (!string.IsNullOrEmpty(tracerHome))
        {
            Response.Write(tracerHome);
        }

        Response.End();

    }
</script>