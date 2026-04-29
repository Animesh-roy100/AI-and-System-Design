import sys
from langchain_core.messages import HumanMessage
from agent import agent_app

def print_stream(stream):
    """Helper to nicely print the streaming state updates from LangGraph."""
    for s in stream:
        for node_name, state_update in s.items():
            print(f"--- [Node Executed: {node_name}] ---")
            
            # Print if a category was assigned
            if "category" in state_update:
                print(f"Assigned Category: {state_update['category'].upper()}")
                
            # Parse messages appended by this node
            if "messages" in state_update:
                last_msg = state_update["messages"][-1]
                
                # Check for textual responses
                if last_msg.content:
                    # Depending on the type, print the right source
                    if last_msg.type == "ai":
                        print(f"🤖 AI: {last_msg.content}")
                    elif last_msg.type == "tool":
                        print(f"🛠️ Tool Result: {last_msg.content}")
                
                # Check if the LLM invoked a tool call
                if getattr(last_msg, "tool_calls", None):
                    tool_names = [tc['name'] for tc in last_msg.tool_calls]
                    print(f"⚡ LLM Requested Action: {tool_names}")
            print()

def run_agent(query: str):
    print("="*60)
    print(f"👤 User Request: {query}")
    print("="*60)
    
    # Initialize the inputs to the state graph
    inputs = {"messages": [HumanMessage(content=query)]}
    
    # Execute the graph -> it yields updates at every super-step.
    # We use stream_mode="updates" so we get exactly the deltas each node returns.
    for chunk in agent_app.stream(inputs, stream_mode="updates"):
        print_stream([chunk])
        
if __name__ == "__main__":
    queries = [
        # Technical Intent (will trigger troubleshoot_technical_issue)
        "My device X-100 is throwing an E404 error code, what does that mean?",
        
        # Billing Intent (will trigger process_refund)
        "Hi, I was overcharged by $15.50 for order #8849. Please give me a refund.",
        
        # General Intent (no tools, directly answered)
        "What are your working hours?"
    ]
    
    for q in queries:
        run_agent(q)
