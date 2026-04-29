from langchain_core.tools import tool

@tool
def process_refund(order_id: str, amount: float) -> str:
    """
    Use this tool to process a refund for a customer. 
    It requires an order ID and the refund amount.
    """
    # In a real system, this would call Stripe, Adyen, or specific eCommerce APIs
    print(f"\n[Mock API Call] Processing refund of ${amount} for order {order_id}...")
    return f"Successfully processed refund of ${amount} for order {order_id}."

@tool
def troubleshoot_technical_issue(device_model: str, error_code: str) -> str:
    """
    Use this tool to look up technical documentation or troubleshooting steps based on a device model and error code.
    """
    # In a real system, this would trigger a Vector Database (RAG) search over documentation
    print(f"\n[Vector DB Search] Searching for {error_code} on {device_model} documentation...")
    if "E404" in error_code.upper():
        return f"Error {error_code} for {device_model}: The device could not connect to WiFi. Suggest factory resetting the network module."
    return "Ensure the device is securely plugged in and check for software updates from the settings menu."

# Bind our tools in a list to attach them to the agent
SUPPORT_TOOLS = [process_refund, troubleshoot_technical_issue]
